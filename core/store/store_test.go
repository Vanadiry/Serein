package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigDefault(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Serein.Host != "127.0.0.1" {
		t.Fatalf("host: %s", cfg.Serein.Host)
	}
	if cfg.Serein.Port != 12510 {
		t.Fatalf("port: %d", cfg.Serein.Port)
	}
	if cfg.Serein.Concurrency != 8 {
		t.Fatalf("concurrency: %d", cfg.Serein.Concurrency)
	}
}

func TestConfigRoundTrip(t *testing.T) {
	dir := t.TempDir()
	cfg := DefaultConfig()
	cfg.Serein.Port = 9999
	if err := SaveConfig(dir, cfg); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Serein.Port != 9999 {
		t.Fatalf("port: %d", loaded.Serein.Port)
	}
}

func TestParseRuleFile(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "test.toml")
	content := `
[info]
app_id = "test-app"
name = "Test App"
platforms = ["macos"]

[config]
type = "github"
owner = "test-owner"
repo = "test-repo"

[config.macos]
d_position = "darwin"
`
	if err := os.WriteFile(tmp, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	rule, err := ParseRuleFile(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if rule.Info.AppID != "test-app" {
		t.Fatalf("app_id: %s", rule.Info.AppID)
	}
	if rule.Info.Name != "Test App" {
		t.Fatalf("name: %s", rule.Info.Name)
	}

	merged := rule.MergedConfig("macos")
	if merged.Owner != "test-owner" {
		t.Fatalf("owner: %s", merged.Owner)
	}
	if merged.DPosition != "darwin" {
		t.Fatalf("d_position: %v", merged.DPosition)
	}
}

func TestParseRuleWithPreRequest(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "test.toml")
	content := `
[info]
app_id = "pre-test"
name = "Pre Test"
platforms = ["windows"]

[pre_request.001]
url = "https://ex.com"
type = "html_selector"
position = { selector = ".latest a", attr = "href" }

[config]
type = "json"

[config.windows]
v_position = ["version"]
d_position = ["url"]
`
	if err := os.WriteFile(tmp, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	rule, err := ParseRuleFile(tmp)
	if err != nil {
		t.Fatal(err)
	}
	chain := rule.PreRequestChain("windows")
	if len(chain) != 1 {
		t.Fatalf("chain len: %d", len(chain))
	}
	if chain[0].URL != "https://ex.com" {
		t.Fatalf("url: %s", chain[0].URL)
	}
}

func TestConfigMerge(t *testing.T) {
	rule := Rule{
		Config: PlatConfig{
			Type: "json",
			VPosition: []any{"shared_version"},
		},
		Platforms: map[string]PlatConfig{
			"macos": {
				URL: "https://mac.example.com",
				DPosition: []any{"mac_dl"},
			},
		},
	}
	merged := rule.MergedConfig("macos")
	if merged.URL != "https://mac.example.com" {
		t.Fatalf("url: %s", merged.URL)
	}
	if merged.Type != "json" {
		t.Fatalf("type: %s", merged.Type)
	}
	// shared VPosition should be preserved
	vp, ok := merged.VPosition.([]any)
	if !ok || len(vp) != 1 || vp[0] != "shared_version" {
		t.Fatalf("vposition: %v", merged.VPosition)
	}
}

func TestTrackerPlatformPriority(t *testing.T) {
	entry := TrackerEntry{AppID: "x", Platforms: []string{"macos"}}
	cfgPlatforms := []string{"macos", "windows"}
	result := PlatformsFor(entry, cfgPlatforms)
	if len(result) != 1 || result[0] != "macos" {
		t.Fatalf("got %v", result)
	}
}

func TestTrackerFallbackToConfig(t *testing.T) {
	entry := TrackerEntry{AppID: "x"}
	cfgPlatforms := []string{"linux"}
	result := PlatformsFor(entry, cfgPlatforms)
	if len(result) != 1 || result[0] != "linux" {
		t.Fatalf("got %v", result)
	}
}

func TestUserDataRoundTrip(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "user"), 0755)
	ud := UserData{
		"abc": {"macos": "1.0", "windows": "2.0"},
	}
	if err := SaveUserData(dir, ud); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadUserData(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded["abc"]["macos"] != "1.0" {
		t.Fatalf("got %s", loaded["abc"]["macos"])
	}
}
