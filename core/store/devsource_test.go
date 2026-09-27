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
        "files": {"A.toml": "1", "Gone.toml": "1"}
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

// rule_source_dev 只支持本地路径：远端地址会因子源 URL 拼接方式而静默失效，
// 与其事后看到难懂的报错，不如直接拒绝
func TestValidateDevSourceRejectsRemote(t *testing.T) {
	for _, p := range []string{
		"http://example.com/_source.json",
		"https://example.com/_source.json",
		"HTTP://EXAMPLE.COM/_source.json",
	} {
		_, err := ValidateDevSource(p, nil)
		if err == nil {
			t.Errorf("%s 应被拒绝", p)
			continue
		}
		if !strings.Contains(err.Error(), "只支持本地路径") {
			t.Errorf("%s 错误信息应说明只支持本地路径: %v", p, err)
		}
	}
}
