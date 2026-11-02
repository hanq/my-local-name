package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"my-local-name/internal/applog"
	"my-local-name/internal/config"
	"my-local-name/internal/mdns"
)

// AppService is the only object bound to the frontend. It orchestrates the
// mDNS engine, persisted settings, autostart, events and the system tray.
type AppService struct {
	app    *application.App
	window *application.WebviewWindow
	tray   *trayUI

	engine *mdns.Engine
	logger *applog.Logger

	opsMu sync.Mutex
	cfgMu sync.RWMutex
	cfg   config.Config

	autostartLaunch bool
}

// NewAppService loads configuration and constructs the mDNS engine.
func NewAppService() *AppService {
	cfg, err := config.Load()
	s := &AppService{cfg: cfg}

	dir, dirErr := config.Dir()
	if dirErr == nil {
		s.logger = applog.New(dir, func(entry applog.Entry) {
			s.emitLog(entry)
		})
	} else {
		s.logger = &applog.Logger{}
	}
	if err != nil {
		s.logger.Log("warn", err.Error())
	}

	port := 5353
	s.engine = mdns.New(mdns.Options{
		Port:    port,
		OnState: s.handleEngineState,
		OnLog:   func(level mdns.LogLevel, message string) { s.logger.Log(string(level), message) },
	})
	return s
}

// ServiceStartup is called by Wails once the application is running.
func (s *AppService) ServiceStartup(_ context.Context, _ application.ServiceOptions) error {
	s.logger.Log("info", fmt.Sprintf("MY LOCAL NAME %s 启动（%s）", Version, platformName()))

	cfg := s.cfgSnapshot()
	s.logger.Log("info", fmt.Sprintf("配置：%s.%s，接口 %v，自启广播 %v", cfg.Prefix, cfg.Suffix, cfg.Interfaces, cfg.AutoBroadcast))

	if s.autostartLaunch && cfg.AutoBroadcast {
		go func() {
			time.Sleep(500 * time.Millisecond)
			s.opsMu.Lock()
			_, err := s.startLocked(cfg.Prefix, cfg.Suffix)
			s.opsMu.Unlock()
			if err != nil {
				s.logger.Log("error", "自启广播失败："+err.Error())
				s.ShowWindow()
			}
		}()
	}

	s.emitState()
	return nil
}

// GetState returns the full snapshot used by the UI.
func (s *AppService) GetState() AppState {
	return s.buildState(s.engine.Status())
}

// GetLogs returns the in-memory log history.
func (s *AppService) GetLogs() []LogLine {
	entries := s.logger.Snapshot()
	out := make([]LogLine, 0, len(entries))
	for _, e := range entries {
		out = append(out, LogLine{ID: e.ID, Time: e.Time, Level: e.Level, Message: e.Message})
	}
	return out
}

// StartBroadcast validates the name, saves it, then starts probing.
func (s *AppService) StartBroadcast(prefix, suffix string) (AppState, error) {
	s.opsMu.Lock()
	defer s.opsMu.Unlock()
	return s.startLocked(prefix, suffix)
}

func (s *AppService) startLocked(prefix, suffix string) (AppState, error) {
	name, err := mdns.NormalizeName(prefix, suffix)
	if err != nil {
		return s.GetState(), err
	}

	s.cfgMu.RLock()
	state := s.engine.Status().State
	cfg := s.cfg.Clone()
	s.cfgMu.RUnlock()
	if isActiveState(string(state)) {
		return s.GetState(), fmt.Errorf("广播已经启动，请先停止")
	}

	cfg.Prefix = name.Prefix
	cfg.Suffix = name.Suffix
	if saveErr := s.saveConfig(cfg); saveErr != nil {
		s.logger.Log("warn", saveErr.Error())
	}

	if err := s.engine.Start(name.Prefix, name.Suffix, cfg.Interfaces); err != nil {
		st := s.GetState()
		s.emitStateValue(st)
		return st, err
	}
	st := s.GetState()
	s.emitStateValue(st)
	return st, nil
}

// ToggleBroadcast starts probing or stops the active responder. It powers the
// F9 shortcut and the tray menu.
func (s *AppService) ToggleBroadcast() (AppState, error) {
	s.opsMu.Lock()
	defer s.opsMu.Unlock()

	if isActiveState(string(s.engine.Status().State)) {
		s.engine.Stop()
		st := s.GetState()
		s.emitStateValue(st)
		return st, nil
	}
	cfg := s.cfgSnapshot()
	return s.startLocked(cfg.Prefix, cfg.Suffix)
}

// StopBroadcast sends goodbye packets and returns to idle.
func (s *AppService) StopBroadcast() (AppState, error) {
	s.opsMu.Lock()
	defer s.opsMu.Unlock()
	s.engine.Stop()
	st := s.GetState()
	s.emitStateValue(st)
	return st, nil
}

// ApplyName validates and saves a name without starting the responder.
func (s *AppService) ApplyName(prefix, suffix string) (AppState, error) {
	s.opsMu.Lock()
	defer s.opsMu.Unlock()

	name, err := mdns.NormalizeName(prefix, suffix)
	if err != nil {
		return s.GetState(), err
	}
	if isActiveState(string(s.engine.Status().State)) {
		return s.GetState(), fmt.Errorf("广播中不能修改名字，请先停止")
	}

	cfg := s.cfgSnapshot()
	cfg.Prefix = name.Prefix
	cfg.Suffix = name.Suffix
	if err := s.saveConfig(cfg); err != nil {
		return s.GetState(), err
	}
	st := s.GetState()
	s.emitStateValue(st)
	return st, nil
}

// UseSuggestedName adopts the conflict suggestion as the saved prefix.
func (s *AppService) UseSuggestedName() (AppState, error) {
	s.opsMu.Lock()
	defer s.opsMu.Unlock()

	st := s.engine.Status()
	if st.Suggestion == "" {
		return s.GetState(), fmt.Errorf("当前没有可用的建议名")
	}
	cfg := s.cfgSnapshot()
	cfg.Prefix = st.Suggestion
	if err := s.saveConfig(cfg); err != nil {
		return s.GetState(), err
	}
	out := s.GetState()
	s.emitStateValue(out)
	return out, nil
}

// ListInterfaces returns selectable interfaces with their current IPv4 addresses.
func (s *AppService) ListInterfaces() []InterfaceOption {
	all, err := mdns.ListInterfaces()
	if err != nil {
		s.logger.Log("warn", err.Error())
		return []InterfaceOption{}
	}
	return s.interfaceOptions(all, s.cfgSnapshot().Interfaces)
}

// SetInterfaces persists the interface selection and restarts broadcasting if needed.
func (s *AppService) SetInterfaces(indexes []int) (AppState, error) {
	s.opsMu.Lock()
	defer s.opsMu.Unlock()

	all, err := mdns.ListInterfaces()
	if err != nil {
		return s.GetState(), err
	}
	available := make(map[int]bool, len(all))
	for _, iface := range all {
		available[iface.Index] = true
	}
	seen := make(map[int]bool)
	var valid []int
	for _, idx := range indexes {
		if !available[idx] || seen[idx] {
			continue
		}
		seen[idx] = true
		valid = append(valid, idx)
	}
	if len(valid) == 0 {
		return s.GetState(), fmt.Errorf("请至少选择一个网络接口")
	}
	if len(valid) == len(all) {
		valid = []int{}
	}
	sort.Ints(valid)

	cfg := s.cfgSnapshot()
	wasActive := isActiveState(string(s.engine.Status().State))
	if wasActive {
		s.engine.Stop()
	}
	cfg.Interfaces = valid
	if err := s.saveConfig(cfg); err != nil {
		return s.GetState(), err
	}
	if wasActive {
		if err := s.engine.Start(cfg.Prefix, cfg.Suffix, cfg.Interfaces); err != nil {
			st := s.GetState()
			s.emitStateValue(st)
			return st, err
		}
	}
	st := s.GetState()
	s.emitStateValue(st)
	return st, nil
}

// SetAutostart registers or removes the login autostart entry.
func (s *AppService) SetAutostart(enabled bool) (bool, error) {
	s.opsMu.Lock()
	defer s.opsMu.Unlock()

	if s.app == nil {
		return false, fmt.Errorf("应用尚未就绪")
	}
	var err error
	if enabled {
		err = s.app.Autostart.EnableWithOptions(application.AutostartOptions{
			Identifier: "cn.hanq.mylocalname",
			Arguments:  []string{"--autostart"},
		})
	} else {
		err = s.app.Autostart.Disable()
	}
	if err != nil {
		s.logger.Log("error", "设置开机自启失败："+err.Error())
	} else {
		s.logger.Log("info", fmt.Sprintf("开机自启已%s", enabledLabel(enabled)))
	}

	status, statusErr := s.app.Autostart.Status()
	s.emitStateValue(s.buildState(s.engine.Status()))
	if statusErr != nil {
		return err == nil, err
	}
	return status.Enabled, err
}

// SetAutoBroadcast toggles broadcasting automatically when launched at login.
func (s *AppService) SetAutoBroadcast(enabled bool) (AppState, error) {
	return s.updateConfig(func(cfg *config.Config) { cfg.AutoBroadcast = enabled })
}

// SetStartMinimized toggles starting hidden in the tray.
func (s *AppService) SetStartMinimized(enabled bool) (AppState, error) {
	return s.updateConfig(func(cfg *config.Config) { cfg.StartMinimized = enabled })
}

func (s *AppService) updateConfig(mut func(*config.Config)) (AppState, error) {
	s.opsMu.Lock()
	defer s.opsMu.Unlock()

	cfg := s.cfgSnapshot()
	mut(&cfg)
	if err := s.saveConfig(cfg); err != nil {
		return s.GetState(), err
	}
	st := s.GetState()
	s.emitStateValue(st)
	return st, nil
}

// CopyText writes text to the system clipboard.
func (s *AppService) CopyText(text string) {
	if s.app != nil {
		s.app.Clipboard.SetText(text)
	}
}

// GetDiagnostics returns commands and paths useful for troubleshooting.
func (s *AppService) GetDiagnostics() Diagnostics {
	cfg := s.cfgSnapshot()
	st := s.engine.Status()
	prefix, suffix := cfg.Prefix, cfg.Suffix
	if st.Prefix != "" {
		prefix, suffix = st.Prefix, st.Suffix
	}
	name, err := mdns.NormalizeName(prefix, suffix)
	fqdn := prefix + "." + suffix
	if err == nil {
		fqdn = name.FQDN
	}

	ifaces := make([]string, 0, 4)
	if all, err := mdns.ListInterfaces(); err == nil {
		for _, iface := range all {
			addrs := make([]string, 0, len(iface.Addresses))
			for _, ip := range iface.Addresses {
				addrs = append(addrs, ip.String())
			}
			ifaces = append(ifaces, fmt.Sprintf("%s · %s", iface.Name, strings.Join(addrs, ", ")))
		}
	}

	cfgPath, _ := config.Path()
	notes := []string{
		"mDNS 组播地址 224.0.0.251，端口 5353/UDP。",
		"停止广播后系统可能缓存记录最多 120 秒，验证前可清理 DNS 缓存。",
	}
	if !strings.EqualFold(suffix, "local") {
		notes = append(notes,
			"自定义后缀在该后缀被 mDNS 客户端接受时才可解析；Windows/macOS 原生解析器通常只处理 .local。")
	}
	if runtime.GOOS == "windows" {
		notes = append(notes,
			"首次运行若出现防火墙提示，请允许「专用网络」的 UDP 5353 入站，否则其他设备可能无法解析。")
	}

	return Diagnostics{
		FQDN:           fqdn,
		Port:           portText(5353),
		Group:          "224.0.0.251",
		ConfigPath:     cfgPath,
		LogPath:        s.logger.Path(),
		Interfaces:     ifaces,
		Commands:       resolverCommands(fqdn),
		Notes:          notes,
		SuffixStandard: strings.EqualFold(suffix, "local"),
	}
}

// HideWindow hides the main window to the tray.
func (s *AppService) HideWindow() {
	if s.window != nil {
		s.window.Hide()
	}
}

// ShowWindow shows and focuses the main window.
func (s *AppService) ShowWindow() {
	if s.window == nil {
		return
	}
	s.window.Show()
	s.window.Restore()
	s.window.Focus()
}

// MinimiseWindow minimises the main window.
func (s *AppService) MinimiseWindow() {
	if s.window != nil {
		s.window.Minimise()
	}
}

// OpenLogFolder reveals the directory that holds the config and log files.
func (s *AppService) OpenLogFolder() {
	dir, err := config.Dir()
	if err != nil {
		return
	}
	_ = os.MkdirAll(dir, 0o755)
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("explorer.exe", dir)
	case "darwin":
		cmd = exec.Command("open", dir)
	default:
		cmd = exec.Command("xdg-open", dir)
	}
	if err := cmd.Start(); err != nil {
		s.logger.Log("warn", "打开日志目录失败："+err.Error())
	}
}

// Quit stops the responder (with goodbye) and exits the application.
func (s *AppService) Quit() {
	s.engine.Stop()
	s.logger.Log("info", "应用退出")
	s.logger.Close()
	if s.app != nil {
		s.app.Quit()
	}
}

func (s *AppService) shutdown() {
	s.engine.Stop()
	s.logger.Close()
}

func (s *AppService) handleEngineState(st mdns.Status) {
	s.emitStateValue(s.buildState(st))
}

func (s *AppService) emitState() {
	s.emitStateValue(s.buildState(s.engine.Status()))
}

func (s *AppService) emitStateValue(st AppState) {
	if s.app != nil {
		s.app.Event.Emit(EventStateUpdate, st)
	}
	if s.tray != nil {
		s.tray.refresh(st)
	}
}

func (s *AppService) emitLog(entry applog.Entry) {
	if s.app == nil {
		return
	}
	s.app.Event.Emit(EventLogLine, LogLine{
		ID: entry.ID, Time: entry.Time, Level: entry.Level, Message: entry.Message,
	})
}

func (s *AppService) cfgSnapshot() config.Config {
	s.cfgMu.RLock()
	defer s.cfgMu.RUnlock()
	return s.cfg.Clone()
}

func (s *AppService) saveConfig(cfg config.Config) error {
	if err := cfg.Save(); err != nil {
		s.logger.Log("error", err.Error())
		return err
	}
	s.cfgMu.Lock()
	s.cfg = cfg.Clone()
	s.cfgMu.Unlock()
	return nil
}

func (s *AppService) interfaceOptions(all []mdns.Interface, selected []int) []InterfaceOption {
	selectedSet := make(map[int]bool, len(selected))
	for _, idx := range selected {
		selectedSet[idx] = true
	}
	autoAll := len(selected) == 0
	out := make([]InterfaceOption, 0, len(all))
	for _, iface := range all {
		addrs := make([]string, 0, len(iface.Addresses))
		for _, ip := range iface.Addresses {
			addrs = append(addrs, ip.String())
		}
		out = append(out, InterfaceOption{
			Index:     iface.Index,
			Name:      iface.Name,
			Addresses: addrs,
			Selected:  autoAll || selectedSet[iface.Index],
		})
	}
	return out
}

func (s *AppService) buildState(st mdns.Status) AppState {
	cfg := s.cfgSnapshot()

	prefix, suffix := st.Prefix, st.Suffix
	if prefix == "" {
		prefix, suffix = cfg.Prefix, cfg.Suffix
	}
	if suffix == "" {
		suffix = "local"
	}
	fqdn := st.FQDN
	if fqdn == "" {
		if name, err := mdns.NormalizeName(prefix, suffix); err == nil {
			fqdn = name.FQDN
			prefix, suffix = name.Prefix, name.Suffix
		}
	}

	all, err := mdns.ListInterfaces()
	if err != nil && len(st.Interfaces) == 0 {
		s.logger.Log("warn", err.Error())
	}
	interfaces := s.interfaceOptions(all, cfg.Interfaces)
	selected := make([]int, 0, len(interfaces))
	for _, iface := range interfaces {
		if iface.Selected {
			selected = append(selected, iface.Index)
		}
	}

	autostartEnabled, autostartSupported, strategy := s.autostartState()

	addresses := make([]string, 0, len(st.Addresses))
	addresses = append(addresses, st.Addresses...)
	state := AppState{
		Status:             string(st.State),
		Prefix:             prefix,
		Suffix:             suffix,
		FQDN:               fqdn,
		Addresses:          addresses,
		Interfaces:         interfaces,
		SelectedInterfaces: selected,
		Error:              st.Error,
		Suggestion:         st.Suggestion,
		QueriesAnswered:    st.QueriesAnswered,
		AnnouncementsSent:  st.AnnouncementsSent,
		ConflictsDetected:  st.ConflictsDetected,
		AutostartEnabled:   autostartEnabled,
		AutostartSupported: autostartSupported,
		AutostartStrategy:  strategy,
		AutoBroadcast:      cfg.AutoBroadcast,
		StartMinimized:     cfg.StartMinimized,
		Port:               5353,
		StandardSuffix:     strings.EqualFold(suffix, "local"),
		TestCommand:        strings.Join(resolverCommands(fqdn), "  ·  "),
		Version:            Version,
		Platform:           platformName(),
	}
	if !st.Since.IsZero() {
		state.Since = st.Since.UnixMilli()
	}
	if !st.StartedAt.IsZero() {
		state.StartedAt = st.StartedAt.UnixMilli()
	}
	if !st.LastAnnounceAt.IsZero() {
		state.LastAnnounceAt = st.LastAnnounceAt.UnixMilli()
	}
	return state
}

func enabledLabel(enabled bool) string {
	if enabled {
		return "开启"
	}
	return "关闭"
}

func (s *AppService) autostartState() (enabled, supported bool, strategy string) {
	if s.app == nil {
		return false, false, ""
	}
	status, err := s.app.Autostart.Status()
	if err != nil {
		return false, false, ""
	}
	return status.Enabled, true, string(status.Strategy)
}
