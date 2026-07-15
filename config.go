package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

/*
	config.go

	Persistent configuration for the Route Debugger plugin.
	Stored as a single config.json next to the plugin binary, following the
	same load/save-with-mutex pattern as the uptime-neko plugin.
*/

const CONFIG_PATH = "./config.json"

// CaptureMode decides what happens to a request that matches a rule.
type CaptureMode string

const (
	// ModeIntercept terminates the request and returns the debug dump to the
	// client instead of proxying it upstream. Full request (incl. body) is logged.
	ModeIntercept CaptureMode = "intercept"
	// ModeTap logs the request metadata to the dashboard but still forwards it
	// upstream, so the client receives the real response. Body is not captured.
	ModeTap CaptureMode = "tap"
)

// CaptureRule describes which requests to capture and how.
//
// Pattern is matched against "<host><path>" (e.g. "my.example.com/test/foo").
// It supports glob wildcards: * matches any run of characters and ? matches a
// single character. Matching is anchored at the start (prefix semantics), so
// "*/debug/" captures /debug/* on any host and "example.com/api" captures the
// whole /api tree on that host.
type CaptureRule struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Pattern     string      `json:"pattern"`
	Mode        CaptureMode `json:"mode"`
	PrettyPrint bool        `json:"pretty_print"` // intercept mode: HTML page vs plain text
	Enabled     bool        `json:"enabled"`
}

// Config is the whole persisted state.
type Config struct {
	Rules           []*CaptureRule `json:"rules"`
	LogLimit        int            `json:"log_limit"`         // max captures kept in memory
	BodyPreviewSize int            `json:"body_preview_size"` // max body bytes captured
}

const (
	DefaultLogLimit        = 200
	DefaultBodyPreviewSize = 4096
)

// ConfigManager wraps Config with safe concurrent access + persistence.
type ConfigManager struct {
	mu  sync.RWMutex
	cfg *Config
}

func defaultConfig() *Config {
	return &Config{
		Rules:           []*CaptureRule{},
		LogLimit:        DefaultLogLimit,
		BodyPreviewSize: DefaultBodyPreviewSize,
	}
}

// LoadConfig reads config.json, falling back to defaults.
func LoadConfig() *ConfigManager {
	cm := &ConfigManager{cfg: defaultConfig()}
	data, err := os.ReadFile(CONFIG_PATH)
	if err != nil {
		fmt.Printf("[route-debugger] no config found, using defaults: %s\n", err.Error())
		return cm
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		fmt.Printf("[route-debugger] failed to parse config, using defaults: %s\n", err.Error())
		return cm
	}
	// Normalize / backfill defaults
	if c.Rules == nil {
		c.Rules = []*CaptureRule{}
	}
	if c.LogLimit <= 0 {
		c.LogLimit = DefaultLogLimit
	}
	if c.BodyPreviewSize <= 0 {
		c.BodyPreviewSize = DefaultBodyPreviewSize
	}
	for _, r := range c.Rules {
		if r.Mode != ModeIntercept && r.Mode != ModeTap {
			r.Mode = ModeIntercept
		}
	}
	cm.cfg = &c
	return cm
}

// Save persists the current config to disk.
func (cm *ConfigManager) Save() error {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.saveLocked()
}

func (cm *ConfigManager) saveLocked() error {
	f, err := os.Create(CONFIG_PATH)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(cm.cfg)
}

// Read gives read access to the config under a read lock.
func (cm *ConfigManager) Read(fn func(c *Config)) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	fn(cm.cfg)
}

// Update mutates the config under a write lock and persists it.
func (cm *ConfigManager) Update(fn func(c *Config)) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	fn(cm.cfg)
	return cm.saveLocked()
}

// MatchRule returns the first enabled rule whose pattern matches the given
// "<host><path>" target, or nil if none match.
func (cm *ConfigManager) MatchRule(target string) *CaptureRule {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	for _, r := range cm.cfg.Rules {
		if !r.Enabled {
			continue
		}
		if matchPattern(r.Pattern, target) {
			return r
		}
	}
	return nil
}

// --- helpers ---

// newID returns a short random hex id.
func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
