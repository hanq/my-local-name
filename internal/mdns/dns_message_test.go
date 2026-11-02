package mdns

import (
	"encoding/binary"
	"net"
	"strings"
	"testing"
)

func TestMessageRoundTrip(t *testing.T) {
	q := NewQuery("Printer.Local", TypeA, true)
	q.ID = 0x1234
	raw, err := q.Pack()
	if err != nil {
		t.Fatalf("pack query: %v", err)
	}
	parsed, err := ParseMessage(raw)
	if err != nil {
		t.Fatalf("parse query: %v", err)
	}
	if parsed.ID != 0x1234 {
		t.Fatalf("id = %#x, want 0x1234", parsed.ID)
	}
	if len(parsed.Questions) != 1 {
		t.Fatalf("questions = %d, want 1", len(parsed.Questions))
	}
	question := parsed.Questions[0]
	if question.Name != "printer.local" {
		t.Fatalf("name = %q", question.Name)
	}
	if question.Type != TypeA || question.QClass() != ClassIN {
		t.Fatalf("type/class = %d/%d", question.Type, question.QClass())
	}
	if !question.UnicastResponse() {
		t.Fatal("QU bit was not preserved")
	}

	resp := NewResponse(0, []Question{question})
	resp.Answers = append(resp.Answers,
		ARecord("printer.local", net.IPv4(192, 168, 1, 9), 120, true),
		ARecord("printer.local", net.IPv4(10, 0, 0, 3), 120, true),
	)
	resp.Additionals = append(resp.Additionals, ARecord("other.local", net.IPv4(10, 0, 0, 4), 120, false))
	raw, err = resp.Pack()
	if err != nil {
		t.Fatalf("pack response: %v", err)
	}
	parsed, err = ParseMessage(raw)
	if err != nil {
		t.Fatalf("parse response: %v", err)
	}
	if !parsed.IsResponse() {
		t.Fatal("QR bit not set")
	}
	if len(parsed.Answers) != 2 || len(parsed.Additionals) != 1 {
		t.Fatalf("answers/additionals = %d/%d", len(parsed.Answers), len(parsed.Additionals))
	}
	if got := ARecordIP(parsed.Answers[0]).String(); got != "192.168.1.9" {
		t.Fatalf("ip = %s", got)
	}
	if !parsed.Answers[0].CacheFlush() || parsed.Answers[0].RClass() != ClassIN {
		t.Fatal("cache-flush class bit not preserved")
	}
	if parsed.Answers[0].TTL != 120 {
		t.Fatalf("ttl = %d", parsed.Answers[0].TTL)
	}
	if parsed.Additionals[0].CacheFlush() {
		t.Fatal("cache-flush should be false")
	}
}

func TestParseCompressedName(t *testing.T) {
	// Header: one question, one answer.
	b := []byte{0, 0, 0x84, 0, 0, 1, 0, 1, 0, 0, 0, 0}
	b = append(b, packNameOrFail(t, "printer.local")...)
	b = append(b, 0, 1, 0, 1) // A, IN
	b = append(b, 0xC0, 0x0C) // pointer to offset 12
	b = append(b, 0, 1, 0x80, 1, 0, 0, 0, 120, 0, 4, 192, 168, 1, 9)

	msg, err := ParseMessage(b)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(msg.Answers) != 1 {
		t.Fatalf("answers = %d", len(msg.Answers))
	}
	if msg.Answers[0].Name != "printer.local" {
		t.Fatalf("compressed name = %q", msg.Answers[0].Name)
	}
	if got := ARecordIP(msg.Answers[0]).String(); got != "192.168.1.9" {
		t.Fatalf("ip = %s", got)
	}
}

func TestParseMalformedDoesNotPanic(t *testing.T) {
	cases := map[string][]byte{
		"empty":           {},
		"short header":    {0, 0, 0, 0, 0, 1},
		"label overflow":  {0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 64},
		"truncated qtype": append(append([]byte{0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0}, byte(3)), []byte("abc")...),
		"pointer loop":    {0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0xC0, 0x0C, 0, 1, 0, 1},
		"pointer forward": {0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0xC0, 0x20, 0, 1, 0, 1},
		"rdlen overflow":  {0, 0, 0x84, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 1, 0, 1, 0, 0, 0, 120, 0, 9},
		"bad question":    {0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0x40, 0, 0, 1, 0, 1},
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseMessage(data); err == nil {
				t.Fatalf("expected an error for %v", data)
			}
		})
	}
}

func TestPackNameValidation(t *testing.T) {
	bad := []string{
		"a..b",
		strings.Repeat("a", 64) + ".local",
		strings.Repeat("a.", 130) + "local",
	}
	for _, name := range bad {
		if _, err := packName(name); err == nil {
			t.Fatalf("expected error for %q", name)
		}
	}
	if _, err := packName("ok.local."); err != nil {
		t.Fatalf("trailing dot should be accepted: %v", err)
	}
}

func TestNewQueryNormalizesName(t *testing.T) {
	q := NewQuery("MyBox.Local.", TypeANY, false)
	if got := q.Questions[0].Name; got != "mybox.local" {
		t.Fatalf("name = %q", got)
	}
	if q.Questions[0].UnicastResponse() {
		t.Fatal("QU bit should not be set")
	}
}

func TestFlagsConstants(t *testing.T) {
	var hdr [4]byte
	binary.BigEndian.PutUint16(hdr[0:2], NewResponse(0, nil).Flags)
	if hdr[0]&0x80 == 0 {
		t.Fatal("expected QR bit in response flags")
	}
}

func packNameOrFail(t *testing.T, name string) []byte {
	t.Helper()
	b, err := packName(name)
	if err != nil {
		t.Fatalf("pack name %q: %v", name, err)
	}
	return b
}
