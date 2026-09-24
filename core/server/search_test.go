package server

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/vanadiry/serein/core/store"
)

func TestSearchHandler(t *testing.T) {
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, "tracker"), 0755)
	os.MkdirAll(filepath.Join(home, "rules"), 0755)
	os.WriteFile(filepath.Join(home, "tracker", "a.toml"),
		[]byte("display_name = \"A\"\n[[tracker]]\napp_id = \"Firefox\"\n[[tracker]]\napp_id = \"VLC\"\n"), 0644)
	os.WriteFile(filepath.Join(home, "rules", "Firefox.toml"),
		[]byte("[info]\napp_id = \"Firefox\"\nname = \"Firefox\"\ndescription = \"浏览器\"\nplatforms = [\"macos\"]\n"), 0644)

	s := &Server{home: home, config: store.Config{}}
	s.config.Tracker.Platforms = []string{"macos"}

	search := func(q string) ([]map[string]any, []map[string]any) {
		rec := httptest.NewRecorder()
		s.handleSearch(rec, httptest.NewRequest("GET", "/api/search?q="+q, nil))
		var out struct {
			Apps  []map[string]any `json:"apps"`
			Rules []map[string]any `json:"rules"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out.Apps, out.Rules
	}

	// 描述匹配
	apps, rules := search("浏览")
	if len(apps) != 1 || apps[0]["app_id"] != "Firefox" || len(rules) != 1 {
		t.Fatalf("描述匹配 apps=%v rules=%v", apps, rules)
	}
	// 大小写不敏感
	if a, _ := search("firefox"); len(a) != 1 {
		t.Fatalf("name 匹配失败: %v", a)
	}
	// 无规则的 app 也能按 app_id 命中
	apps, rules = search("vlc")
	if len(apps) != 1 || apps[0]["app_id"] != "VLC" || len(rules) != 0 {
		t.Fatalf("app_id 匹配 apps=%v rules=%v", apps, rules)
	}
	// 空 q
	if a, r := search(""); len(a) != 0 || len(r) != 0 {
		t.Fatalf("空 q 应为空: %v %v", a, r)
	}
}
