package config

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestNeverOverwriteConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	_, created, err := LoadOrInit(path)
	if err != nil || !created {
		t.Fatalf("%v %v", created, err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadOrInit(path); err == nil {
		t.Fatal("default secrets accepted")
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(before, after) {
		t.Fatal("existing config overwritten")
	}
	c := defaultConfig()
	c.AdminSecret = "  CHANGEME_STRONG_SECRET  "
	if err := c.Validate(); err == nil {
		t.Fatal("padded default accepted")
	}
}

func TestStrictValidationAndMissingDefaults(t *testing.T) {
	for _, raw := range []string{
		`{"admin_secret":"test-admin","user_secret":"test-user"}`,
		`{"admin_secret":"test-admin","user_secret":"test-user","admin_bind_cidrs":[]}`,
	} {
		path := filepath.Join(t.TempDir(), "config.json")
		os.WriteFile(path, []byte(raw), 0600)
		cfg, err := Load(path)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.SearchRequestDelaySeconds != 5 || cfg.SearchRequestJitterSeconds != 2 {
			t.Fatal("missing pacing keys did not inherit defaults")
		}
		if after, err := os.ReadFile(path); err != nil || string(after) != raw {
			t.Fatal("loading missing defaults modified the config file", err)
		}
	}
	for _, field := range []string{`"admin_bind_cidrs":null`, `"admin_bind_cidrs":["broken"]`, `"http_write_timeout_sec":0`, `"typo":1`, `"daily_ingest_time":"25:00"`, `"feed_max_age_days":-1`, `"score_evidence_window_hours":0`, `"date_refresh_max_per_run":501`, `"feed_freshness_decay_days":-1`} {
		path := filepath.Join(t.TempDir(), "config.json")
		os.WriteFile(path, []byte(`{"admin_secret":"test-admin","user_secret":"test-user",`+field+`}`), 0600)
		if _, err := Load(path); err == nil {
			t.Fatalf("accepted %s", field)
		}
	}
}

func TestSearchPacingValidation(t *testing.T) {
	c := defaultConfig()
	c.AdminSecret, c.UserSecret = "test-admin", "test-user"
	for _, pair := range [][2]int{{-1, 0}, {3601, 0}, {0, -1}, {0, 601}} {
		c.SearchRequestDelaySeconds, c.SearchRequestJitterSeconds = pair[0], pair[1]
		if c.Validate() == nil {
			t.Fatal("accepted invalid pacing", pair)
		}
	}
	c.SearchRequestDelaySeconds, c.SearchRequestJitterSeconds = 0, 0
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestExampleMatchesDefaults(t *testing.T) {
	b, err := os.ReadFile("../../config.example.json")
	if err != nil {
		t.Fatal(err)
	}
	var example Config
	if err := json.Unmarshal(b, &example); err != nil {
		t.Fatal(err)
	}
	want, _ := json.Marshal(defaultConfig())
	got, _ := json.Marshal(example)
	if !bytes.Equal(want, got) {
		t.Fatal("example differs from generated defaults")
	}
}
