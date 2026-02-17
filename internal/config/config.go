package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const defaultAdminSecret = "CHANGEME_STRONG_SECRET"
const defaultUserSecret = "CHANGEME_USER_SECRET"

type Config struct {
	ListenAddress         string   `json:"listen_address"`
	EnableTLS             bool     `json:"enable_tls"`
	TLSCertPath           string   `json:"tls_cert_path"`
	TLSKeyPath            string   `json:"tls_key_path"`
	UserName              string   `json:"user_name"`
	UserSecret            string   `json:"user_secret"`
	AdminSecret           string   `json:"admin_secret"`
	AdminBindCIDRs        []string `json:"admin_bind_cidrs"`
	DatabasePath          string   `json:"database_path"`
	DailyIngestTime       string   `json:"daily_ingest_time"`
	IngestIntervalMinutes int      `json:"ingest_interval_minutes"`
	SearxngInstances      []string `json:"searxng_instances"`
	PerQueryDelaySeconds  int      `json:"per_query_delay_seconds"`
	PerQueryJitterSeconds int      `json:"per_query_jitter_seconds"`
	HTTPReadTimeoutSec    int      `json:"http_read_timeout_sec"`
	HTTPWriteTimeoutSec   int      `json:"http_write_timeout_sec"`
	HTTPIdleTimeoutSec    int      `json:"http_idle_timeout_sec"`
	MaxBodyBytes          int64    `json:"max_body_bytes"`
	DefaultBatchSize      int      `json:"default_batch_size"`
	FeedMinScore          float64  `json:"feed_min_score"`
	AutoHideBelowScore    float64  `json:"auto_hide_below_score"`
	CullUnreadDays        int      `json:"cull_unread_days"`
	CullMaxScore          float64  `json:"cull_max_score"`
}

func defaultConfig() Config {
	return Config{
		ListenAddress:         ":8443",
		EnableTLS:             true,
		TLSCertPath:           "/etc/letsencrypt/live/example.com/fullchain.pem",
		TLSKeyPath:            "/etc/letsencrypt/live/example.com/privkey.pem",
		UserName:              "discover",
		UserSecret:            defaultUserSecret,
		AdminSecret:           defaultAdminSecret,
		AdminBindCIDRs:        []string{"127.0.0.1/32", "::1/128", "192.168.0.0/16", "10.0.0.0/8"},
		DatabasePath:          "discover.db",
		DailyIngestTime:       "07:30",
		IngestIntervalMinutes: 120,
		SearxngInstances:      []string{"http://localhost:8888"},
		PerQueryDelaySeconds:  5,
		PerQueryJitterSeconds: 5,
		HTTPReadTimeoutSec:    10,
		HTTPWriteTimeoutSec:   20,
		HTTPIdleTimeoutSec:    60,
		MaxBodyBytes:          1 << 20,
		DefaultBatchSize:      10,
		FeedMinScore:          1,
		AutoHideBelowScore:    1,
		CullUnreadDays:        30,
		CullMaxScore:          0,
	}
}

func LoadOrInit(path string) (Config, bool, error) {
	path = filepath.Clean(path)
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		cfg := defaultConfig()
		if err := writeConfig(path, cfg); err != nil {
			return Config{}, false, err
		}
		return cfg, true, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return Config{}, false, fmt.Errorf("read config: %w", err)
	}
	cfg := defaultConfig()
	if err := json.Unmarshal(b, &cfg); err != nil {
		return Config{}, false, fmt.Errorf("parse config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, false, err
	}
	return cfg, false, nil
}

func writeConfig(path string, cfg Config) error {
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(path, b, 0o600)
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.ListenAddress) == "" {
		return errors.New("listen_address is required")
	}
	if c.EnableTLS {
		if c.TLSCertPath == "" || c.TLSKeyPath == "" {
			return errors.New("tls_cert_path and tls_key_path are required when enable_tls=true")
		}
	}
	if c.AdminSecret == "" || c.AdminSecret == defaultAdminSecret {
		return errors.New("admin_secret must be set to a non-default value")
	}
	if strings.TrimSpace(c.UserName) == "" {
		return errors.New("user_name is required")
	}
	if c.UserSecret == "" || c.UserSecret == defaultUserSecret {
		return errors.New("user_secret must be set to a non-default value")
	}
	if len(c.SearxngInstances) == 0 {
		return errors.New("at least one searxng_instances entry is required")
	}
	if c.PerQueryDelaySeconds < 0 || c.PerQueryDelaySeconds > 3600 {
		return errors.New("per_query_delay_seconds out of range")
	}
	if c.PerQueryJitterSeconds < 0 || c.PerQueryJitterSeconds > 600 {
		return errors.New("per_query_jitter_seconds out of range")
	}
	if c.DefaultBatchSize <= 0 || c.DefaultBatchSize > 100 {
		return errors.New("default_batch_size must be 1..100")
	}
	if c.IngestIntervalMinutes < 0 || c.IngestIntervalMinutes > 24*60 {
		return errors.New("ingest_interval_minutes out of range")
	}
	if c.IngestIntervalMinutes > 0 && c.IngestIntervalMinutes < 5 {
		return errors.New("ingest_interval_minutes must be >=5 when enabled")
	}
	if c.FeedMinScore < -100 || c.FeedMinScore > 1000 {
		return errors.New("feed_min_score out of range")
	}
	if c.AutoHideBelowScore < -100 || c.AutoHideBelowScore > 1000 {
		return errors.New("auto_hide_below_score out of range")
	}
	if c.MaxBodyBytes <= 0 {
		return errors.New("max_body_bytes must be positive")
	}
	return nil
}

func MissingKeys(path string) ([]string, error) {
	b, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	expected := []string{
		"listen_address",
		"enable_tls",
		"tls_cert_path",
		"tls_key_path",
		"user_name",
		"user_secret",
		"admin_secret",
		"admin_bind_cidrs",
		"database_path",
		"daily_ingest_time",
		"ingest_interval_minutes",
		"searxng_instances",
		"per_query_delay_seconds",
		"per_query_jitter_seconds",
		"http_read_timeout_sec",
		"http_write_timeout_sec",
		"http_idle_timeout_sec",
		"max_body_bytes",
		"default_batch_size",
		"feed_min_score",
		"auto_hide_below_score",
		"cull_unread_days",
		"cull_max_score",
	}
	missing := make([]string, 0, len(expected))
	for _, key := range expected {
		if _, ok := raw[key]; !ok {
			missing = append(missing, key)
		}
	}
	slices.Sort(missing)
	return missing, nil
}
