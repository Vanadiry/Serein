package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateDevSource(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "v-test")
	os.MkdirAll(sub, 0755)

	os.WriteFile(filepath.Join(root, "_source.json"), []byte(`{
        "source_id": "DevTest", "type": "list",
        "files": ["v-test/_source.json", "v-missing/_source.json"]
    }`), 0644)
	os.WriteFile(filepath.Join(sub, "_source.json"), []byte(`{
        "source_id": "v-test", "type": "rules", "version": 1,
        "files": ["A.toml", "Gone.toml"]
    }`), 0644)
	os.WriteFile(filepath.Join(sub, "A.toml"), []byte(`[info]
app_id = "A"
name = "A"
platforms = ["macos"]
[config]
type = "json"
url = "https://x"
`), 0644)
	// 多余文件（未被 source 列出）
	os.WriteFile(filepath.Join(sub, "B.toml"), []byte("x = 1"), 0644)

	issues, err := ValidateDevSource(filepath.Join(root, "_source.json"), nil)
	if err != nil {
		t.Fatal(err)
	}
	joined := ""
	for _, is := range issues {
		joined += is.Message + "\n"
	}
	for _, want := range []string{
		"v-missing/_source.json: 文件不存在",
		"v-test/Gone.toml: 文件不存在",
		"v-test/B.toml: 未在该规则源中列出",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("缺少问题: %q\n得到:\n%s", want, joined)
		}
	}
	// A.toml 合法，不应有 A.toml 的问题
	if strings.Contains(joined, "A.toml") {
		t.Errorf("A.toml 不应有问题:\n%s", joined)
	}
}
