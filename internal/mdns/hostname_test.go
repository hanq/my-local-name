package mdns

import (
	"strings"
	"testing"
)

func TestNormalizeName(t *testing.T) {
	tests := []struct {
		prefix, suffix string
		wantFQDN       string
		wantSuffix     string
	}{
		{"hanq-pi", "local", "hanq-pi.local", "local"},
		{"Hanq-Pi", ".Local", "hanq-pi.local", "local"},
		{"mybox", "", "mybox.local", "local"},
		{"mybox", ".home.arpa.", "mybox.home.arpa", "home.arpa"},
		{" a1 ", " .lan ", "a1.lan", "lan"},
	}
	for _, tt := range tests {
		got, err := NormalizeName(tt.prefix, tt.suffix)
		if err != nil {
			t.Fatalf("NormalizeName(%q,%q): %v", tt.prefix, tt.suffix, err)
		}
		if got.FQDN != tt.wantFQDN || got.Suffix != tt.wantSuffix {
			t.Fatalf("NormalizeName(%q,%q) = %+v, want fqdn=%q suffix=%q",
				tt.prefix, tt.suffix, got, tt.wantFQDN, tt.wantSuffix)
		}
	}
}

func TestNormalizeNameRejects(t *testing.T) {
	cases := []struct{ prefix, suffix, want string }{
		{"", "local", "不能为空"},
		{"a b", "local", "空格"},
		{"a.b", "local", "点号"},
		{"-box", "local", "连字符"},
		{"box-", "local", "连字符"},
		{"box_name", "local", "只能包含"},
		{"box", "a..b", "不能为空"},
		{"box", "lo cal", "只能包含"},
		{strings.Repeat("a", 64), "local", "过长"},
		{strings.Repeat("a", 63), strings.Repeat("b", 63) + "." + strings.Repeat("c", 63) + "." + strings.Repeat("d", 63), "过长"},
	}
	for _, tt := range cases {
		_, err := NormalizeName(tt.prefix, tt.suffix)
		if err == nil {
			t.Fatalf("NormalizeName(%q,%q) should fail", tt.prefix, tt.suffix)
		}
		if !strings.Contains(err.Error(), tt.want) {
			t.Fatalf("error %q does not contain %q", err, tt.want)
		}
	}
}

func TestSuggestName(t *testing.T) {
	if got := SuggestName("hanq", "local"); got != "hanq-2" {
		t.Fatalf("suggestion = %q", got)
	}
	long := strings.Repeat("a", 63)
	got := SuggestName(long, "local")
	if len(got) > 63 {
		t.Fatalf("suggestion too long: %d", len(got))
	}
	if !strings.HasSuffix(got, "-2") {
		t.Fatalf("suggestion = %q", got)
	}
	if _, err := NormalizeName(got, "local"); err != nil {
		t.Fatalf("suggestion is invalid: %v", err)
	}
}

func TestSanitizePrefix(t *testing.T) {
	tests := map[string]string{
		"DESKTOP-ABC123":        "desktop-abc123",
		"  my laptop  ":         "my-laptop",
		"foo.bar":               "foo",
		"---":                   "my-device",
		"你好":                    "my-device",
		"a_b__c":                "a-b-c",
		strings.Repeat("x", 80): strings.Repeat("x", 63),
	}
	for in, want := range tests {
		if got := SanitizePrefix(in); got != want {
			t.Fatalf("SanitizePrefix(%q) = %q, want %q", in, got, want)
		}
	}
}
