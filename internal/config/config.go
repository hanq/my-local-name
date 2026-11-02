package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"my-local-name/internal/mdns"
)

// Config is the persisted user configuration.
type Config struct {
	Prefix         string `json:"prefix"`
	Suffix         string `json:"suffix"`
	Interfaces     []int  `json:"interfaces"`
	AutoBroadcast  bool   `json:"autoBroadcast"`
	StartMinimized bool   `json:"startMinimized"`

	extra map[string]json.RawMessage
}

// Dir returns the per-user configuration directory for the application.
func Dir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("获取用户配置目录失败: %w", err)
	}
	return filepath.Join(base, "my-local-name"), nil
}

// Path returns the config file location.
func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

// Default builds a first-run configuration. The "-lan" suffix avoids claiming
// the operating system's own mDNS hostname, which would always conflict.
func Default() Config {
	return Config{
		Prefix:     defaultPrefix(),
		Suffix:     "local",
		Interfaces: []int{},
	}
}

func defaultPrefix() string {
	hostname, _ := os.Hostname()
	base := mdns.SanitizePrefix(hostname)
	const marker = "-lan"
	if len(base)+len(marker) <= 63 {
		return base + marker
	}
	base = strings.TrimRight(base[:63-len(marker)], "-")
	if base == "" {
		base = "my-device"
	}
	return base + marker
}

// Load reads the config file. A missing file is not an error; a corrupted file
// returns the defaults together with an error so the caller can surface it.
func Load() (Config, error) {
	path, err := Path()
	if err != nil {
		return Default(), err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Default(), nil
		}
		return Default(), fmt.Errorf("读取配置失败: %w", err)
	}
	cfg := Default()
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return Default(), fmt.Errorf("配置文件损坏，已使用默认值: %w", err)
	}
	if cfg.extra == nil {
		cfg.extra = make(map[string]json.RawMessage)
	}
	var all map[string]json.RawMessage
	if json.Unmarshal(raw, &all) == nil {
		for _, known := range []string{"prefix", "suffix", "interfaces", "autoBroadcast", "startMinimized"} {
			delete(all, known)
		}
		cfg.extra = all
	}
	cfg.normalize()
	return cfg, nil
}

func (c *Config) normalize() {
	name, err := mdns.NormalizeName(c.Prefix, c.Suffix)
	if err != nil {
		c.Prefix = defaultPrefix()
		c.Suffix = "local"
	} else {
		c.Prefix = name.Prefix
		c.Suffix = name.Suffix
	}
	seen := make(map[int]bool)
	cleaned := c.Interfaces[:0]
	for _, idx := range c.Interfaces {
		if idx <= 0 || seen[idx] {
			continue
		}
		seen[idx] = true
		cleaned = append(cleaned, idx)
	}
	sort.Ints(cleaned)
	c.Interfaces = cleaned
}

// Clone returns a copy safe to mutate.
func (c Config) Clone() Config {
	out := c
	out.Interfaces = append([]int(nil), c.Interfaces...)
	if c.extra != nil {
		out.extra = make(map[string]json.RawMessage, len(c.extra))
		for k, v := range c.extra {
			out.extra[k] = v
		}
	}
	return out
}

// Save writes the config atomically, preserving unknown JSON fields.
func (c Config) Save() error {
	dir, err := Dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("创建配置目录失败: %w", err)
	}
	if c.Interfaces == nil {
		c.Interfaces = []int{}
	}

	known, err := json.Marshal(c)
	if err != nil {
		return fmt.Errorf("编码配置失败: %w", err)
	}
	merged := make(map[string]json.RawMessage)
	if err := json.Unmarshal(known, &merged); err != nil {
		return fmt.Errorf("编码配置失败: %w", err)
	}
	for k, v := range c.extra {
		if _, exists := merged[k]; !exists {
			merged[k] = v
		}
	}
	raw, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		return fmt.Errorf("编码配置失败: %w", err)
	}
	raw = append(raw, '\n')

	path := filepath.Join(dir, "config.json")
	tmp, err := os.CreateTemp(dir, ".config-*.json.tmp")
	if err != nil {
		return fmt.Errorf("写入配置失败: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }
	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("写入配置失败: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("写入配置失败: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		cleanup()
		return fmt.Errorf("写入配置失败: %w", err)
	}
	return nil
}
