package mdns

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/net/ipv4"
)

// State is the lifecycle state of the responder.
type State string

const (
	StateIdle         State = "idle"
	StateProbing      State = "probing"
	StateAnnouncing   State = "announcing"
	StateBroadcasting State = "broadcasting"
	StateConflict     State = "conflict"
	StateError        State = "error"
)

// LogLevel is used for callback logging.
type LogLevel string

const (
	LevelInfo  LogLevel = "info"
	LevelWarn  LogLevel = "warn"
	LevelError LogLevel = "error"
)

// ErrAlreadyRunning is returned when Start is called while a responder is active.
var ErrAlreadyRunning = errors.New("广播已经启动")

// ConflictError is returned when another host already owns the name.
type ConflictError struct {
	FQDN       string
	Suggestion string
	Reason     string
}

func (e *ConflictError) Error() string {
	if e.Suggestion != "" {
		return fmt.Sprintf("名称冲突：%s；建议改用 %s", e.Reason, e.Suggestion)
	}
	return "名称冲突：" + e.Reason
}

// Stats holds counters shown in the UI.
type Stats struct {
	QueriesAnswered   int64
	AnnouncementsSent int64
	ConflictsDetected int64
	LastAnnounceAt    time.Time
}

// Status is a point-in-time snapshot of the responder.
type Status struct {
	State             State
	Prefix            string
	Suffix            string
	FQDN              string
	Addresses         []string
	Interfaces        []string
	Error             string
	Suggestion        string
	Since             time.Time
	StartedAt         time.Time
	QueriesAnswered   int64
	AnnouncementsSent int64
	ConflictsDetected int64
	LastAnnounceAt    time.Time
}

// Options configures the engine. Zero values pick production defaults.
type Options struct {
	Port             int
	TTL              uint32
	ProbeCount       int
	ProbeInterval    time.Duration
	AnnounceInterval time.Duration
	WatchInterval    time.Duration
	OnState          func(Status)
	OnLog            func(level LogLevel, message string)
}

func (o *Options) applyDefaults() {
	if o.Port == 0 {
		o.Port = 5353
	}
	if o.TTL == 0 {
		o.TTL = 120
	}
	if o.ProbeCount == 0 {
		o.ProbeCount = 3
	}
	if o.ProbeInterval == 0 {
		o.ProbeInterval = 250 * time.Millisecond
	}
	if o.AnnounceInterval == 0 {
		o.AnnounceInterval = 60 * time.Second
	}
	if o.WatchInterval == 0 {
		o.WatchInterval = 5 * time.Second
	}
}

type counters struct {
	queriesAnswered   atomic.Int64
	announcementsSent atomic.Int64
	conflictsDetected atomic.Int64
	lastAnnounceUnix  atomic.Int64 // unix nanoseconds
}

type runState struct {
	conn       net.PacketConn
	p4         *ipv4.PacketConn
	ctx        context.Context
	cancel     context.CancelFunc
	conflict   chan string
	wg         sync.WaitGroup
	stopOnce   sync.Once
	group      *net.UDPAddr
	port       int
	ifaceIndex []int
}

// Engine is a single-host mDNS responder that publishes A records for one name.
type Engine struct {
	opts    Options
	opMu    sync.Mutex // serializes Start/Stop
	sendMu  sync.Mutex // serializes socket writes
	mu      sync.Mutex // guards the fields below
	run     *runState
	state   State
	name    Name
	ifaces  []Interface
	addrs   []net.IP
	errMsg  string
	suggest string
	since   time.Time
	started time.Time
	stats   counters
}

// New creates an engine. Callbacks are optional.
func New(opts Options) *Engine {
	opts.applyDefaults()
	return &Engine{opts: opts, state: StateIdle}
}

// Status returns the latest snapshot.
func (e *Engine) Status() Status {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.statusLocked()
}

func (e *Engine) statusLocked() Status {
	st := Status{
		State:             e.state,
		Prefix:            e.name.Prefix,
		Suffix:            e.name.Suffix,
		FQDN:              e.name.FQDN,
		Error:             e.errMsg,
		Suggestion:        e.suggest,
		Since:             e.since,
		StartedAt:         e.started,
		QueriesAnswered:   e.stats.queriesAnswered.Load(),
		AnnouncementsSent: e.stats.announcementsSent.Load(),
		ConflictsDetected: e.stats.conflictsDetected.Load(),
	}
	if last := e.stats.lastAnnounceUnix.Load(); last > 0 {
		st.LastAnnounceAt = time.Unix(0, last)
	}
	for _, ip := range e.addrs {
		st.Addresses = append(st.Addresses, ip.String())
	}
	for _, iface := range e.ifaces {
		st.Interfaces = append(st.Interfaces, iface.Name)
	}
	return st
}

// Start validates the name, probes for conflicts, announces records and then
// keeps answering queries until Stop is called.
func (e *Engine) Start(prefix, suffix string, interfaceIndexes []int) error {
	e.opMu.Lock()
	defer e.opMu.Unlock()

	e.mu.Lock()
	alreadyRunning := e.run != nil
	e.mu.Unlock()
	if alreadyRunning {
		return ErrAlreadyRunning
	}

	name, err := NormalizeName(prefix, suffix)
	if err != nil {
		return err
	}

	all, err := ListInterfaces()
	if err != nil {
		e.setFailed(err.Error())
		return err
	}
	ifaces := FilterInterfaces(all, interfaceIndexes)
	if len(ifaces) == 0 {
		msg := "没有可用的局域网接口，请连接网络后重试"
		e.setFailed(msg)
		return errors.New(msg)
	}
	addrs := Addresses(ifaces)
	if len(addrs) == 0 {
		msg := "所选接口没有可用的 IPv4 地址"
		e.setFailed(msg)
		return errors.New(msg)
	}

	conn, p4, err := openSocket(ifaces, e.opts.Port)
	if err != nil {
		msg := fmt.Sprintf("无法监听 UDP %d：%v", e.opts.Port, err)
		e.setFailed(msg)
		return errors.New(msg)
	}

	ctx, cancel := context.WithCancel(context.Background())
	run := &runState{
		conn:       conn,
		p4:         p4,
		ctx:        ctx,
		cancel:     cancel,
		conflict:   make(chan string, 1),
		group:      &net.UDPAddr{IP: net.IPv4(224, 0, 0, 251), Port: e.opts.Port},
		port:       e.opts.Port,
		ifaceIndex: append([]int(nil), interfaceIndexes...),
	}

	e.mu.Lock()
	e.run = run
	e.state = StateProbing
	e.name = name
	e.ifaces = ifaces
	e.addrs = addrs
	e.errMsg = ""
	e.suggest = ""
	e.started = time.Now()
	e.since = e.started
	e.stats.queriesAnswered.Store(0)
	e.stats.announcementsSent.Store(0)
	e.stats.conflictsDetected.Store(0)
	e.stats.lastAnnounceUnix.Store(0)
	e.mu.Unlock()
	e.emit()

	e.logf(LevelInfo, "开始探测 %s（%d 个接口，%d 个地址）", name.FQDN, len(ifaces), len(addrs))

	run.wg.Add(1)
	go e.readLoop(run)

	if reason := e.probe(run); reason != "" {
		return e.failConflict(run, reason)
	}

	e.setState(StateAnnouncing)
	e.logf(LevelInfo, "探测通过，开始宣告 %s", name.FQDN)
	for i := 0; i < 2; i++ {
		if i > 0 && !sleepCtx(run.ctx, time.Second) {
			return nil
		}
		if reason := drainConflict(run); reason != "" {
			return e.failConflict(run, reason)
		}
		e.announceOnce(run)
	}

	e.setState(StateBroadcasting)
	run.wg.Add(2)
	go e.periodicLoop(run)
	go e.watchLoop(run)
	go e.conflictLoop(run)
	return nil
}

// Stop sends goodbye records, closes the socket and resets the state to idle.
func (e *Engine) Stop() {
	e.opMu.Lock()
	defer e.opMu.Unlock()

	e.mu.Lock()
	run := e.run
	state := e.state
	e.mu.Unlock()

	if run == nil {
		if state != StateIdle {
			e.mu.Lock()
			e.state = StateIdle
			e.errMsg = ""
			e.suggest = ""
			e.started = time.Time{}
			e.since = time.Now()
			e.addrs = nil
			e.stats.queriesAnswered.Store(0)
			e.stats.announcementsSent.Store(0)
			e.stats.lastAnnounceUnix.Store(0)
			e.mu.Unlock()
			e.emit()
		}
		return
	}
	e.stopRun(run, StateIdle, "", "")
}

// failConflict transitions the engine into the conflict state with a suggested
// replacement name. It is used by probing, announcing and the live conflict
// watcher.
func (e *Engine) failConflict(run *runState, reason string) *ConflictError {
	e.mu.Lock()
	name := e.name
	e.mu.Unlock()
	suggestion := SuggestName(name.Prefix, name.Suffix)
	e.stats.conflictsDetected.Add(1)
	e.logf(LevelError, "检测到名称冲突：%s", reason)
	e.stopRun(run, StateConflict, reason, suggestion)
	return &ConflictError{FQDN: name.FQDN, Suggestion: suggestion, Reason: reason}
}

func (e *Engine) probe(run *runState) string {
	for i := 0; i < e.opts.ProbeCount; i++ {
		e.mu.Lock()
		fqdn := e.name.FQDN
		e.mu.Unlock()

		// Probe responses are requested via multicast (no QU bit). On Windows
		// several processes may share port 5353; unicast replies can be
		// delivered to the wrong socket, while multicast reaches all of them.
		query := NewQuery(fqdn, TypeANY, false)
		packet, err := query.Pack()
		if err == nil {
			e.sendMulticast(run, packet)
		}
		if reason := drainConflict(run); reason != "" {
			return reason
		}
		if !sleepCtx(run.ctx, e.opts.ProbeInterval) {
			return ""
		}
		if reason := drainConflict(run); reason != "" {
			return reason
		}
	}
	return ""
}

func (e *Engine) periodicLoop(run *runState) {
	defer run.wg.Done()
	ticker := time.NewTicker(e.opts.AnnounceInterval)
	defer ticker.Stop()
	for {
		select {
		case <-run.ctx.Done():
			return
		case <-ticker.C:
			e.announceOnce(run)
		}
	}
}

func (e *Engine) watchLoop(run *runState) {
	defer run.wg.Done()
	ticker := time.NewTicker(e.opts.WatchInterval)
	defer ticker.Stop()
	for {
		select {
		case <-run.ctx.Done():
			return
		case <-ticker.C:
			e.refreshAddresses(run)
		}
	}
}

func (e *Engine) conflictLoop(run *runState) {
	for {
		select {
		case <-run.ctx.Done():
			return
		case reason := <-run.conflict:
			e.opMu.Lock()
			e.mu.Lock()
			current := e.run == run
			e.mu.Unlock()
			if current {
				e.failConflict(run, reason)
			}
			e.opMu.Unlock()
			return
		}
	}
}

func (e *Engine) refreshAddresses(run *runState) {
	e.mu.Lock()
	indexes := append([]int(nil), run.ifaceIndex...)
	oldAddrs := append([]net.IP(nil), e.addrs...)
	e.mu.Unlock()

	all, err := ListInterfaces()
	if err != nil {
		e.logf(LevelWarn, "刷新网络接口失败：%v", err)
		return
	}
	selected := FilterInterfaces(all, indexes)
	addrs := Addresses(selected)

	e.mu.Lock()
	changed := !sameAddressSet(e.addrs, addrs)
	if changed {
		e.addrs = addrs
		e.ifaces = selected
	}
	e.mu.Unlock()

	if !changed {
		return
	}
	e.logf(LevelInfo, "网络地址变化：%s", describeIPs(oldAddrs, addrs))
	if len(addrs) == 0 {
		e.logf(LevelWarn, "所选接口暂时没有可用 IPv4 地址，等待恢复")
		e.emit()
		return
	}
	e.announceOnce(run)
	e.emit()
}

func (e *Engine) readLoop(run *runState) {
	defer run.wg.Done()
	buf := make([]byte, 9000)
	for {
		n, _, src, err := run.p4.ReadFrom(buf)
		if err != nil {
			if run.ctx.Err() != nil {
				return
			}
			e.logf(LevelWarn, "读取 mDNS 报文失败：%v", err)
			continue
		}
		msg, err := ParseMessage(buf[:n])
		if err != nil {
			continue
		}
		udpSrc, ok := src.(*net.UDPAddr)
		if !ok {
			continue
		}
		if msg.IsResponse() {
			e.handleResponse(run, msg)
		} else {
			e.handleQuery(run, msg, udpSrc)
		}
	}
}

func (e *Engine) handleQuery(run *runState, msg *Message, src *net.UDPAddr) {
	e.mu.Lock()
	state := e.state
	fqdn := e.name.FQDN
	addrs := append([]net.IP(nil), e.addrs...)
	e.mu.Unlock()
	if state != StateAnnouncing && state != StateBroadcasting {
		return
	}
	for _, q := range msg.Questions {
		if !strings.EqualFold(q.Name, fqdn) {
			continue
		}
		class := q.QClass()
		if class != ClassIN && class != ClassANY {
			continue
		}
		if q.Type != TypeA && q.Type != TypeANY {
			continue
		}
		if len(addrs) == 0 {
			return
		}

		legacy := src.Port != run.port
		unicast := legacy || q.UnicastResponse()
		ttl := e.opts.TTL
		var questions []Question
		var id uint16
		if unicast {
			ttl = 10
			id = msg.ID
			questions = msg.Questions
		}
		resp := NewResponse(id, questions)
		for _, ip := range addrs {
			resp.Answers = append(resp.Answers, ARecord(fqdn, ip, ttl, true))
		}
		packet, err := resp.Pack()
		if err != nil {
			return
		}
		if unicast {
			e.sendUnicast(run, packet, src)
		} else {
			e.sendMulticast(run, packet)
		}
		e.stats.queriesAnswered.Add(1)
		e.logf(LevelInfo, "应答 %s 的 %s 查询", src.IP, typeName(q.Type))
		return
	}
}

func (e *Engine) handleResponse(run *runState, msg *Message) {
	e.mu.Lock()
	state := e.state
	fqdn := e.name.FQDN
	addrs := append([]net.IP(nil), e.addrs...)
	e.mu.Unlock()

	if state != StateProbing && state != StateAnnouncing && state != StateBroadcasting {
		return
	}
	records := make([]Record, 0, len(msg.Answers)+len(msg.Additionals))
	records = append(records, msg.Answers...)
	records = append(records, msg.Additionals...)
	for _, r := range records {
		if !strings.EqualFold(r.Name, fqdn) {
			continue
		}
		switch r.Type {
		case TypeA:
			ip := ARecordIP(r)
			if ip == nil {
				continue
			}
			if state == StateProbing {
				e.signalConflict(run, fmt.Sprintf("发现同名设备 %s（%s）", fqdn, ip))
				return
			}
			if !HasAddress(addrs, ip) {
				e.signalConflict(run, fmt.Sprintf("发现同名设备 %s（%s）", fqdn, ip))
				return
			}
		case TypeAAAA:
			if state == StateProbing {
				e.signalConflict(run, fmt.Sprintf("发现同名 IPv6 设备 %s", fqdn))
				return
			}
			e.signalConflict(run, fmt.Sprintf("发现同名 IPv6 设备 %s", fqdn))
			return
		}
	}
}

func (e *Engine) signalConflict(run *runState, reason string) {
	select {
	case run.conflict <- reason:
	default:
	}
}

func (e *Engine) announceOnce(run *runState) {
	e.mu.Lock()
	state := e.state
	fqdn := e.name.FQDN
	addrs := append([]net.IP(nil), e.addrs...)
	e.mu.Unlock()
	if state != StateAnnouncing && state != StateBroadcasting {
		return
	}
	if len(addrs) == 0 {
		return
	}
	resp := NewResponse(0, nil)
	for _, ip := range addrs {
		resp.Answers = append(resp.Answers, ARecord(fqdn, ip, e.opts.TTL, true))
	}
	packet, err := resp.Pack()
	if err != nil {
		return
	}
	e.sendMulticast(run, packet)
	e.stats.announcementsSent.Add(1)
	e.stats.lastAnnounceUnix.Store(time.Now().UnixNano())
}

func (e *Engine) sendGoodbye(run *runState) {
	e.mu.Lock()
	fqdn := e.name.FQDN
	addrs := append([]net.IP(nil), e.addrs...)
	e.mu.Unlock()
	if len(addrs) == 0 {
		return
	}
	resp := NewResponse(0, nil)
	for _, ip := range addrs {
		resp.Answers = append(resp.Answers, ARecord(fqdn, ip, 0, true))
	}
	packet, err := resp.Pack()
	if err != nil {
		return
	}
	e.sendMulticast(run, packet)
	time.Sleep(50 * time.Millisecond)
	e.sendMulticast(run, packet)
	e.logf(LevelInfo, "已发送 goodbye（%s）", fqdn)
}

func (e *Engine) sendMulticast(run *runState, packet []byte) {
	e.sendMu.Lock()
	defer e.sendMu.Unlock()
	if run.ctx.Err() != nil {
		return
	}

	e.mu.Lock()
	ifaces := append([]Interface(nil), e.ifaces...)
	e.mu.Unlock()

	sent := 0
	for _, iface := range ifaces {
		ni, err := net.InterfaceByIndex(iface.Index)
		if err != nil {
			continue
		}
		if err := run.p4.SetMulticastInterface(ni); err != nil {
			continue
		}
		if _, err := run.p4.WriteTo(packet, nil, run.group); err == nil {
			sent++
		}
	}
	if sent == 0 {
		e.logf(LevelWarn, "组播发送失败：没有可用接口")
	}
}

func (e *Engine) sendUnicast(run *runState, packet []byte, dst *net.UDPAddr) {
	e.sendMu.Lock()
	defer e.sendMu.Unlock()
	if run.ctx.Err() != nil {
		return
	}
	_, _ = run.p4.WriteTo(packet, nil, dst)
}

func (e *Engine) stopRun(run *runState, final State, errMsg, suggestion string) {
	run.stopOnce.Do(func() {
		e.mu.Lock()
		claimed := e.state == StateAnnouncing || e.state == StateBroadcasting
		e.mu.Unlock()

		if claimed {
			e.sendGoodbye(run)
		}
		run.cancel()
		_ = run.conn.Close()
		run.wg.Wait()

		e.mu.Lock()
		if e.run == run {
			e.run = nil
		}
		e.state = final
		e.errMsg = errMsg
		e.suggest = suggestion
		e.addrs = nil
		e.ifaces = nil
		e.started = time.Time{}
		e.since = time.Now()
		if final == StateIdle {
			e.stats.queriesAnswered.Store(0)
			e.stats.announcementsSent.Store(0)
			e.stats.lastAnnounceUnix.Store(0)
		}
		e.mu.Unlock()

		if final == StateIdle {
			e.logf(LevelInfo, "广播已停止")
		}
		e.emit()
	})
}

func (e *Engine) setState(state State) {
	e.mu.Lock()
	if e.run == nil {
		e.mu.Unlock()
		return
	}
	e.state = state
	e.since = time.Now()
	e.mu.Unlock()
	e.emit()
}

func (e *Engine) setFailed(message string) {
	e.mu.Lock()
	e.state = StateError
	e.errMsg = message
	e.suggest = ""
	e.addrs = nil
	e.ifaces = nil
	e.started = time.Time{}
	e.since = time.Now()
	e.mu.Unlock()
	e.logf(LevelError, "%s", message)
	e.emit()
}

func (e *Engine) emit() {
	if e.opts.OnState != nil {
		e.opts.OnState(e.Status())
	}
}

func (e *Engine) logf(level LogLevel, format string, args ...any) {
	if e.opts.OnLog != nil {
		e.opts.OnLog(level, fmt.Sprintf(format, args...))
	}
}

func openSocket(ifaces []Interface, port int) (net.PacketConn, *ipv4.PacketConn, error) {
	lc := net.ListenConfig{Control: reuseControl}
	conn, err := lc.ListenPacket(context.Background(), "udp4", fmt.Sprintf("0.0.0.0:%d", port))
	if err != nil {
		return nil, nil, err
	}
	if udp, ok := conn.(*net.UDPConn); ok {
		_ = udp.SetReadBuffer(1 << 20)
	}
	p4 := ipv4.NewPacketConn(conn)
	_ = p4.SetMulticastLoopback(true)

	group := &net.UDPAddr{IP: net.IPv4(224, 0, 0, 251)}
	joined := 0
	for _, iface := range ifaces {
		ni, err := net.InterfaceByIndex(iface.Index)
		if err != nil {
			continue
		}
		if err := p4.JoinGroup(ni, group); err != nil {
			continue
		}
		joined++
	}
	if joined == 0 {
		_ = conn.Close()
		return nil, nil, errors.New("无法加入 mDNS 组播组")
	}
	return conn, p4, nil
}

func drainConflict(run *runState) string {
	select {
	case reason := <-run.conflict:
		return reason
	default:
		return ""
	}
}

func sleepCtx(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func sameAddressSet(a, b []net.IP) bool {
	if len(a) != len(b) {
		return false
	}
	as := make([]string, 0, len(a))
	bs := make([]string, 0, len(b))
	for _, ip := range a {
		as = append(as, ip.String())
	}
	for _, ip := range b {
		bs = append(bs, ip.String())
	}
	sort.Strings(as)
	sort.Strings(bs)
	for i := range as {
		if as[i] != bs[i] {
			return false
		}
	}
	return true
}

func describeIPs(old, next []net.IP) string {
	format := func(ips []net.IP) string {
		parts := make([]string, 0, len(ips))
		for _, ip := range ips {
			parts = append(parts, ip.String())
		}
		if len(parts) == 0 {
			return "无"
		}
		sort.Strings(parts)
		return strings.Join(parts, ",")
	}
	return format(old) + " → " + format(next)
}

func typeName(t uint16) string {
	switch t {
	case TypeA:
		return "A"
	case TypeANY:
		return "ANY"
	case TypeAAAA:
		return "AAAA"
	default:
		return fmt.Sprintf("TYPE%d", t)
	}
}
