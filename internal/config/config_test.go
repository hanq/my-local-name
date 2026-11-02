package config

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"my-local-name/internal/mdns"
)

func setConfigEnv(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("AppData", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)
	return dir
}

func TestDefaultPrefixIsValidAndAvoidsSystemHostname(t *testing.T) {
	cfg := Default()
	if !strings.HasSuffix(cfg.Prefix, "-lan") {
		t.Fatalf("default prefix %q should end in -lan", cfg.Prefix)
	}
	if cfg.Suffix != "local" {
		t.Fatalf("default suffix = %q", cfg.Suffix)
	}
	if _, err := mdns.NormalizeName(cfg.Prefix, cfg.Suffix); err != nil {
		t.Fatalf("default prefix is invalid: %v", err)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	setConfigEnv(t)
	cfg := Default()
	cfg.Prefix = "unit-box"
	cfg.Suffix = "home.arpa"
	cfg.Interfaces = []int{5, 2}
	cfg.AutoBroadcast = true
	cfg.StartMinimized = true
	if err := cfg.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}

	path, err := Path()
	if err != nil {
		t.Fatalf("path: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("decode: %v", err)
	}
	fields["futureField"] = json.RawMessage(`{"enabled":true}`)
	raw, _ = json.Marshal(fields)
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write unknown field: %v", err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.Prefix != "unit-box" || loaded.Suffix != "home.arpa" {
		t.Fatalf("name = %s.%s", loaded.Prefix, loaded.Suffix)
	}
	if len(loaded.Interfaces) != 2 || loaded.Interfaces[0] != 2 || loaded.Interfaces[1] != 5 {
		t.Fatalf("interfaces = %v", loaded.Interfaces)
	}
	if !loaded.AutoBroadcast || !loaded.StartMinimized {
		t.Fatal("boolean settings were not preserved")
	}
	if string(loaded.extra["futureField"]) != `{"enabled":true}` {
		t.Fatalf("unknown field not retained: %v", loaded.extra)
	}

	loaded.Prefix = "second-box"
	if err := loaded.Save(); err != nil {
		t.Fatalf("save again: %v", err)
	}
	raw, _ = os.ReadFile(path)
	fields = nil
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("decode saved: %v", err)
	}
	if _, ok := fields["futureField"]; !ok {
		t.Fatal("unknown field was dropped on save")
	}
}

func TestLoadCorruptFallsBackToDefaults(t *testing.T) {
	setConfigEnv(t)
	path, _ := Path()
	if err := os.MkdirAll(strings.TrimSuffix(path, "config.json"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	cfg, err := Load()
	if err == nil {
		t.Fatal("expected an error for a corrupted config")
	}
	if _, nameErr := mdns.NormalizeName(cfg.Prefix, cfg.Suffix); nameErr != nil {
		t.Fatalf("fallback config is invalid: %v", nameErr)
	}
}

func TestLoadNormalizesInvalidValues(t *testing.T) {
	setConfigEnv(t)
	path, _ := Path()
	if err := os.MkdirAll(strings.TrimSuffix(path, "config.json"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	body := `{"prefix":"Bad Name","suffix":".LAN","interfaces":[3,3,-1,0,7]}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if _, nameErr := mdns.NormalizeName(cfg.Prefix, cfg.Suffix); nameErr != nil {
		t.Fatalf("normalized config invalid: %v", nameErr)
	}
	if len(cfg.Interfaces) != 2 || cfg.Interfaces[0] != 3 || cfg.Interfaces[1] != 7 {
		t.Fatalf("interfaces = %v", cfg.Interfaces)
	}
}
