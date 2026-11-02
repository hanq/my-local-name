package mdns

import (
	"errors"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"
)

const testPort = 15353

func testOptions() Options {
	return Options{
		Port:             testPort,
		ProbeInterval:    80 * time.Millisecond,
		AnnounceInterval: 400 * time.Millisecond,
		WatchInterval:    300 * time.Millisecond,
	}
}

func uniquePrefix(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("my-local-name-test-%d", time.Now().UnixNano()%1_000_000_000)
}

func ask(t *testing.T, name string) (*Message, error) {
	t.Helper()
	conn, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen client: %v", err)
	}
	defer conn.Close()
	query := NewQuery(name, TypeA, true)
	query.ID = 0x4242
	raw, err := query.Pack()
	if err != nil {
		t.Fatalf("pack query: %v", err)
	}
	if _, err := conn.WriteTo(raw, &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: testPort}); err != nil {
		t.Fatalf("send query: %v", err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(700 * time.Millisecond))
	buf := make([]byte, 9000)
	n, _, err := conn.ReadFrom(buf)
	if err != nil {
		return nil, err
	}
	return ParseMessage(buf[:n])
}

func TestEngineStartAnswerStop(t *testing.T) {
	prefix := uniquePrefix(t)
	e := New(testOptions())
	if err := e.Start(prefix, "local", nil); err != nil {
		t.Fatalf("start: %v", err)
	}

	msg, err := ask(t, prefix+".local")
	if err != nil {
		t.Fatalf("query while broadcasting: %v", err)
	}
	if len(msg.Answers) == 0 {
		t.Fatal("expected at least one A record")
	}
	if got := msg.Answers[0].Name; got != prefix+".local" {
		t.Fatalf("answer name = %q", got)
	}
	if got := ARecordIP(msg.Answers[0]); got == nil || !got.IsPrivate() {
		t.Fatalf("unexpected address %v", got)
	}
	if msg.Answers[0].TTL != 10 {
		t.Fatalf("legacy unicast TTL = %d, want 10", msg.Answers[0].TTL)
	}
	if msg.ID != 0x4242 {
		t.Fatalf("response ID = %#x, want echoed 0x4242", msg.ID)
	}

	st := e.Status()
	if st.State != StateBroadcasting {
		t.Fatalf("state = %s", st.State)
	}
	if st.QueriesAnswered < 1 {
		t.Fatalf("queries answered = %d", st.QueriesAnswered)
	}
	if st.AnnouncementsSent < 2 {
		t.Fatalf("announcements = %d", st.AnnouncementsSent)
	}

	e.Stop()
	if st := e.Status(); st.State != StateIdle || len(st.Addresses) != 0 {
		t.Fatalf("after stop: %+v", st)
	}
	if _, err := ask(t, prefix+".local"); err == nil {
		t.Fatal("engine answered after stop")
	}
}

func TestEngineConflict(t *testing.T) {
	prefix := uniquePrefix(t)
	first := New(testOptions())
	if err := first.Start(prefix, "local", nil); err != nil {
		t.Fatalf("first start: %v", err)
	}
	defer first.Stop()

	second := New(testOptions())
	err := second.Start(prefix, "local", nil)
	if err == nil {
		second.Stop()
		t.Fatal("expected a conflict error")
	}
	var conflict *ConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("error type = %T (%v)", err, err)
	}
	if conflict.Suggestion == "" {
		t.Fatal("expected a suggested name")
	}
	st := second.Status()
	if st.State != StateConflict {
		t.Fatalf("state = %s, want conflict", st.State)
	}
	if st.Suggestion != conflict.Suggestion {
		t.Fatalf("status suggestion = %q, error suggestion = %q", st.Suggestion, conflict.Suggestion)
	}
	second.Stop()
	if second.Status().State != StateIdle {
		t.Fatal("conflict state should reset to idle")
	}
}

func TestEngineNoInterfaces(t *testing.T) {
	e := New(testOptions())
	err := e.Start(uniquePrefix(t), "local", []int{-12345})
	if err == nil {
		e.Stop()
		t.Fatal("expected an error for an unknown interface")
	}
	if st := e.Status(); st.State != StateError {
		t.Fatalf("state = %s", st.State)
	}
	e.Stop()
	if st := e.Status(); st.State != StateIdle || st.Error != "" {
		t.Fatalf("after reset: %+v", st)
	}
}

func TestEngineValidationDoesNotChangeState(t *testing.T) {
	e := New(testOptions())
	if err := e.Start("bad name", "local", nil); err == nil {
		e.Stop()
		t.Fatal("expected validation error")
	}
	if st := e.Status(); st.State != StateIdle {
		t.Fatalf("state = %s, want idle", st.State)
	}
}

func TestEngineProgressStates(t *testing.T) {
	var mu sync.Mutex
	var seen []State
	opts := testOptions()
	opts.OnState = func(st Status) {
		mu.Lock()
		seen = append(seen, st.State)
		mu.Unlock()
	}
	e := New(opts)
	prefix := uniquePrefix(t)
	if err := e.Start(prefix, "local", nil); err != nil {
		t.Fatalf("start: %v", err)
	}
	e.Stop()

	mu.Lock()
	defer mu.Unlock()
	want := []State{StateProbing, StateAnnouncing, StateBroadcasting, StateIdle}
	for _, w := range want {
		found := false
		for _, s := range seen {
			if s == w {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("state %s missing from %v", w, seen)
		}
	}
}

func TestSameAddressSet(t *testing.T) {
	a := []net.IP{net.IPv4(10, 0, 0, 2), net.IPv4(192, 168, 1, 2)}
	b := []net.IP{net.IPv4(192, 168, 1, 2), net.IPv4(10, 0, 0, 2)}
	if !sameAddressSet(a, b) {
		t.Fatal("sets should be equal")
	}
	if sameAddressSet(a, b[:1]) {
		t.Fatal("sets should differ")
	}
}
