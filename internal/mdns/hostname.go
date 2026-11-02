package mdns

import (
	"errors"
	"fmt"
	"strings"
)

// Name is a validated mDNS hostname split into prefix and suffix.
type Name struct {
	Prefix string
	Suffix string
	FQDN   string
}

// Validation errors are exposed as plain errors with Chinese messages so the
// UI can show them directly.
var ErrEmptyPrefix = errors.New("名字不能为空")

// NormalizeName validates and normalizes a prefix and suffix.
//
// prefix and suffix are lower-cased. The suffix may be entered with or without
// a leading dot and may contain multiple labels. The returned FQDN never has a
// trailing dot.
func NormalizeName(prefix, suffix string) (Name, error) {
	prefix = strings.ToLower(strings.TrimSpace(prefix))
	suffix = strings.ToLower(strings.TrimSpace(suffix))
	suffix = strings.TrimLeft(suffix, ".")
	suffix = strings.TrimSuffix(suffix, ".")
	if suffix == "" {
		suffix = "local"
	}
	if prefix == "" {
		return Name{}, ErrEmptyPrefix
	}
	if strings.ContainsAny(prefix, " \t\r\n") {
		return Name{}, errors.New("名字不能包含空格")
	}
	if strings.Contains(prefix, ".") {
		return Name{}, errors.New("前缀不能包含点号，请把后面的部分填到后缀里")
	}
	if err := validateLabel(prefix, "前缀"); err != nil {
		return Name{}, err
	}
	for _, part := range strings.Split(suffix, ".") {
		if err := validateLabel(part, "后缀"); err != nil {
			return Name{}, err
		}
	}
	fqdn := prefix + "." + suffix
	if len(fqdn) > maxNameLength {
		return Name{}, fmt.Errorf("完整名字过长（%d 字节，最多 %d）", len(fqdn), maxNameLength)
	}
	return Name{Prefix: prefix, Suffix: suffix, FQDN: fqdn}, nil
}

func validateLabel(label, what string) error {
	if label == "" {
		return fmt.Errorf("%s不能为空", what)
	}
	if len(label) > maxLabelLength {
		return fmt.Errorf("%s过长（最多 %d 字节）", what, maxLabelLength)
	}
	for i, r := range label {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == '-':
			if i == 0 || i == len(label)-1 {
				return fmt.Errorf("%s不能以连字符开头或结尾", what)
			}
		default:
			return fmt.Errorf("%s只能包含小写字母、数字和连字符，发现 %q", what, r)
		}
	}
	return nil
}

// SuggestName returns a conflict-free candidate based on an existing prefix.
// It appends -2, -3, ... while keeping the label within 63 bytes.
func SuggestName(prefix, suffix string) string {
	prefix = strings.TrimSuffix(strings.TrimSpace(strings.ToLower(prefix)), "-")
	if prefix == "" {
		prefix = "my-device"
	}
	for i := 2; i < 1000; i++ {
		suffixPart := fmt.Sprintf("-%d", i)
		base := prefix
		if len(base)+len(suffixPart) > maxLabelLength {
			base = strings.TrimRight(base[:maxLabelLength-len(suffixPart)], "-")
		}
		candidate := base + suffixPart
		if _, err := NormalizeName(candidate, suffix); err == nil {
			return candidate
		}
	}
	return "my-device-2"
}

// SanitizePrefix turns an arbitrary OS hostname into a valid label prefix.
func SanitizePrefix(hostname string) string {
	hostname = strings.ToLower(strings.TrimSpace(hostname))
	if i := strings.IndexByte(hostname, '.'); i >= 0 {
		hostname = hostname[:i]
	}
	var b strings.Builder
	lastDash := false
	for _, r := range hostname {
		valid := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if valid {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash && b.Len() > 0 {
			b.WriteByte('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "my-device"
	}
	if len(out) > maxLabelLength {
		out = strings.TrimRight(out[:maxLabelLength], "-")
	}
	if out == "" {
		return "my-device"
	}
	return out
}
