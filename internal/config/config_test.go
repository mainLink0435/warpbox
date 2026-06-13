package config

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	// Create minimal config with only required field.
	content := []byte("torbox:\n  api_key: \"test-key-123\"\n")
	tmp := t.TempDir() + "/config.yml"
	if err := os.WriteFile(tmp, content, 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(tmp)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Check required field.
	if cfg.TorBox.APIKey != "test-key-123" {
		t.Errorf("api_key = %q, want %q", cfg.TorBox.APIKey, "test-key-123")
	}

	// Check defaults.
	if cfg.Server.ListenAddr != ":1412" {
		t.Errorf("listen_addr = %q, want %q", cfg.Server.ListenAddr, ":1412")
	}
	if cfg.Cache.CDNURLTTLMinutes != 120 {
		t.Errorf("cdn_url_ttl_minutes = %d, want %d", cfg.Cache.CDNURLTTLMinutes, 120)
	}
	if cfg.Throttle.RequestsPerMinute != 250 {
		t.Errorf("requests_per_minute = %d, want %d", cfg.Throttle.RequestsPerMinute, 250)
	}
	if cfg.Logging.Format != "text" {
		t.Errorf("log_format = %q, want %q", cfg.Logging.Format, "text")
	}
	if cfg.Logging.Level != "info" {
		t.Errorf("log_level = %q, want %q", cfg.Logging.Level, "info")
	}
	if cfg.Sync.IntervalMinutes != 5 {
		t.Errorf("sync_interval = %d, want %d", cfg.Sync.IntervalMinutes, 5)
	}
}

func TestLoadCustomValues(t *testing.T) {
	content := []byte(`
torbox:
  api_key: "custom-key"
  base_url: "https://custom.api/torbox"

server:
  listen_addr: "127.0.0.1:9000"
  webdav_root: "/files"

cache:
  cdn_url_ttl_minutes: 240

throttle:
  requests_per_minute: 100

logging:
  format: "json"

sync:
  interval_minutes: 10
`)
	tmp := t.TempDir() + "/config.yml"
	if err := os.WriteFile(tmp, content, 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(tmp)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.TorBox.APIKey != "custom-key" {
		t.Errorf("api_key = %q", cfg.TorBox.APIKey)
	}
	if cfg.Server.ListenAddr != "127.0.0.1:9000" {
		t.Errorf("listen_addr = %q", cfg.Server.ListenAddr)
	}
	if cfg.Server.WebDAVRoot != "/files" {
		t.Errorf("webdav_root = %q", cfg.Server.WebDAVRoot)
	}
	if cfg.Cache.CDNURLTTLMinutes != 240 {
		t.Errorf("cdn_url_ttl_minutes = %d", cfg.Cache.CDNURLTTLMinutes)
	}
	if cfg.Throttle.RequestsPerMinute != 100 {
		t.Errorf("requests_per_minute = %d", cfg.Throttle.RequestsPerMinute)
	}
	if cfg.Logging.Format != "json" {
		t.Errorf("log_format = %q", cfg.Logging.Format)
	}
	if cfg.Sync.IntervalMinutes != 10 {
		t.Errorf("sync_interval = %d", cfg.Sync.IntervalMinutes)
	}
}

func TestLoadMissingAPIKey(t *testing.T) {
	content := []byte("torbox:\n  base_url: \"https://test\"\n")
	tmp := t.TempDir() + "/config.yml"
	if err := os.WriteFile(tmp, content, 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(tmp)
	if err == nil {
		t.Fatal("expected error for missing api_key, got nil")
	}
}

func TestLoadInvalidFile(t *testing.T) {
	_, err := Load("/nonexistent/path/config.yml")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestLoadInvalidYAML(t *testing.T) {
	content := []byte("torbox:\n  api_key: [invalid yaml\n")
	tmp := t.TempDir() + "/config.yml"
	if err := os.WriteFile(tmp, content, 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(tmp)
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input string
		want  string // slog.Level.String()
	}{
		{"debug", "DEBUG"},
		{"info", "INFO"},
		{"warn", "WARN"},
		{"error", "ERROR"},
		{"DEBUG", "DEBUG"},
		{"Info", "INFO"},
	}

	for _, tt := range tests {
		lvl, err := ParseLevel(tt.input)
		if err != nil {
			t.Errorf("ParseLevel(%q): unexpected error: %v", tt.input, err)
			continue
		}
		if lvl.String() != tt.want {
			t.Errorf("ParseLevel(%q) = %s, want %s", tt.input, lvl, tt.want)
		}
	}
}

func TestParseLevelInvalid(t *testing.T) {
	_, err := ParseLevel("verbose")
	if err == nil {
		t.Error("expected error for invalid level 'verbose'")
	}
	_, err = ParseLevel("")
	if err == nil {
		t.Error("expected error for empty level")
	}
}

func TestLoadInvalidLevel(t *testing.T) {
	content := []byte("torbox:\n  api_key: \"key\"\nlogging:\n  level: \"verbose\"\n")
	tmp := t.TempDir() + "/config.yml"
	if err := os.WriteFile(tmp, content, 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(tmp)
	if err == nil {
		t.Fatal("expected error for invalid logging level, got nil")
	}
}

func TestParseFormat(t *testing.T) {
	tests := []struct {
		input string
		valid bool
	}{
		{"text", true},
		{"json", true},
		{"TEXT", true},
		{"Json", true},
		{"", false},
		{"blah", false},
		{"xml", false},
	}

	for _, tt := range tests {
		err := ParseFormat(tt.input)
		if tt.valid && err != nil {
			t.Errorf("ParseFormat(%q): unexpected error: %v", tt.input, err)
		}
		if !tt.valid && err == nil {
			t.Errorf("ParseFormat(%q): expected error, got nil", tt.input)
		}
	}
}

func TestLoadInvalidFormat(t *testing.T) {
	content := []byte("torbox:\n  api_key: \"key\"\nlogging:\n  format: \"xml\"\n")
	tmp := t.TempDir() + "/config.yml"
	if err := os.WriteFile(tmp, content, 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(tmp)
	if err == nil {
		t.Fatal("expected error for invalid logging format, got nil")
	}
}

func TestLoadInvalidCDNURLTTL(t *testing.T) {
	tests := []struct {
		name  string
		value int
	}{
		{"negative", -100},
		{"too_high", 2000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			yaml := fmt.Sprintf("torbox:\n  api_key: \"key\"\ncache:\n  cdn_url_ttl_minutes: %d\n", tt.value)
			tmp := t.TempDir() + "/config.yml"
			if err := os.WriteFile(tmp, []byte(yaml), 0644); err != nil {
				t.Fatal(err)
			}

			_, err := Load(tmp)
			if err == nil {
				t.Errorf("expected error for cdn_url_ttl_minutes=%d, got nil", tt.value)
			}
		})
	}
}

func TestLoadInvalidRequestsPerMinute(t *testing.T) {
	tests := []struct {
		name  string
		value int
	}{
		{"negative", -5},
		{"too_high", 5000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			yaml := fmt.Sprintf("torbox:\n  api_key: \"key\"\nthrottle:\n  requests_per_minute: %d\n", tt.value)
			tmp := t.TempDir() + "/config.yml"
			if err := os.WriteFile(tmp, []byte(yaml), 0644); err != nil {
				t.Fatal(err)
			}

			_, err := Load(tmp)
			if err == nil {
				t.Errorf("expected error for requests_per_minute=%d, got nil", tt.value)
			}
		})
	}
}

func TestLoadInvalidSyncInterval(t *testing.T) {
	yaml := "torbox:\n  api_key: \"key\"\nsync:\n  interval_minutes: -1"
	tmp := t.TempDir() + "/config.yml"
	if err := os.WriteFile(tmp, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(tmp)
	if err == nil {
		t.Fatal("expected error for negative sync interval, got nil")
	}
}

func TestLoadInvalidSyncLimit(t *testing.T) {
	yaml := "torbox:\n  api_key: \"key\"\nsync:\n  limit: 200000"
	tmp := t.TempDir() + "/config.yml"
	if err := os.WriteFile(tmp, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(tmp)
	if err == nil {
		t.Fatal("expected error for oversized sync limit, got nil")
	}
}

func TestLoadInvalidStatsInterval(t *testing.T) {
	yaml := "torbox:\n  api_key: \"key\"\nstats:\n  interval_seconds: 5"
	tmp := t.TempDir() + "/config.yml"
	if err := os.WriteFile(tmp, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(tmp)
	if err == nil {
		t.Fatal("expected error for too-small stats interval, got nil")
	}
}

func TestLoadInvalidRetentionHours(t *testing.T) {
	yaml := "torbox:\n  api_key: \"key\"\nstats:\n  retention_hours: -1"
	tmp := t.TempDir() + "/config.yml"
	if err := os.WriteFile(tmp, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(tmp)
	if err == nil {
		t.Fatal("expected error for negative retention hours, got nil")
	}
}

func TestLoadInvalidChartMinutes(t *testing.T) {
	yaml := "torbox:\n  api_key: \"key\"\nstats:\n  chart_minutes: -1"
	tmp := t.TempDir() + "/config.yml"
	if err := os.WriteFile(tmp, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(tmp)
	if err == nil {
		t.Fatal("expected error for negative chart minutes, got nil")
	}
}

func TestGenerateTemplateCreatesFile(t *testing.T) {
	dir := t.TempDir()

	// GenerateTemplate now reads config.yml.example from beside the target path.
	example := dir + "/config.yml.example"
	if err := os.WriteFile(example, []byte("torbox:\n  api_key: \"YOUR_API_KEY_HERE\"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	tmp := dir + "/config.yml"
	created, err := GenerateTemplate(tmp)
	if err != nil {
		t.Fatalf("GenerateTemplate failed: %v", err)
	}
	if !created {
		t.Fatal("expected created=true for new file")
	}

	// Verify the file exists and is non-empty.
	info, err := os.Stat(tmp)
	if err != nil {
		t.Fatalf("stat failed: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("generated config is empty")
	}

	// Verify it contains the placeholder.
	data, err := os.ReadFile(tmp)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if !strings.Contains(string(data), "YOUR_API_KEY_HERE") {
		t.Error("generated config should contain the API key placeholder")
	}
}

func TestGenerateTemplateExistingFile(t *testing.T) {
	tmp := t.TempDir() + "/config.yml"
	if err := os.WriteFile(tmp, []byte("existing"), 0644); err != nil {
		t.Fatal(err)
	}

	created, err := GenerateTemplate(tmp)
	if err != nil {
		t.Fatalf("GenerateTemplate failed: %v", err)
	}
	if created {
		t.Fatal("expected created=false for existing file")
	}
}

func TestGenerateTemplateBadPath(t *testing.T) {
	_, err := GenerateTemplate("/nonexistent/dir/config.yml")
	if err == nil {
		t.Fatal("expected error for bad path")
	}
}