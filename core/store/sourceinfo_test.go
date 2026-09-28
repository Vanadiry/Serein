package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSourceInfoFilesDispatch(t *testing.T) {
	// type 缺省即 rules：files 必须是对象
	var rules SourceInfo
	if err := json.Unmarshal([]byte(`{"source_id":"s","files":{"a.toml":"1"}}`), &rules); err != nil {
		t.Fatalf("rules 形态解析失败: %v", err)
	}
	if rules.IsList() || rules.Files["a.toml"] != "1" {
		t.Errorf("rules 形态解析错误: %+v", rules)
	}

	// list：files 必须是数组
	var list SourceInfo
	if err := json.Unmarshal([]byte(`{"source_id":"s","type":"list","files":["a/_source.json"]}`), &list); err != nil {
		t.Fatalf("list 形态解析失败: %v", err)
	}
	if !list.IsList() || len(list.SubSources) != 1 || list.SubSources[0] != "a/_source.json" {
		t.Errorf("list 形态解析错误: %+v", list)
	}

	// 形态不匹配必须报错，静默当成空会让整个源同步出一条空规则集
	for _, bad := range []string{
		`{"source_id":"s","files":["a.toml"]}`,
		`{"source_id":"s","type":"list","files":{"a.toml":"1"}}`,
	} {
		var s SourceInfo
		if err := json.Unmarshal([]byte(bad), &s); err == nil {
			t.Errorf("形态不匹配应报错: %s", bad)
		}
	}
}

// list 里的非 <name>/_source.json 条目：剔除 + warn，不阻断其余规则子源（Q2）
func TestValidateSourceFilesList(t *testing.T) {
	var s SourceInfo
	raw := `{"source_id":"S","type":"list","files":["ok/_source.json","also-ok/_source.json","a.toml","x/y/_source.json","_source.json"]}`
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		t.Fatal(err)
	}
	issues, rejected := validateSourceFiles(&s)
	if rejected {
		t.Fatal("结构问题不应导致整体拒绝")
	}
	if len(s.SubSources) != 2 || s.SubSources[0] != "ok/_source.json" || s.SubSources[1] != "also-ok/_source.json" {
		t.Errorf("应只保留合法条目，实际 %v", s.SubSources)
	}
	// 剔除 3 条：规则表条目、多层嵌套、顶层 marker
	if len(issues) != 3 {
		t.Errorf("issues = %d 条, want 3: %+v", len(issues), issues)
	}
	for _, is := range issues {
		if is.Level != "warn" {
			t.Errorf("级别应为 warn: %+v", is)
		}
	}
	if !strings.Contains(issues[0].Message, "已忽略") {
		t.Errorf("提示语应说明已忽略该条目: %s", issues[0].Message)
	}
}

// rules 里含路径的 key 与 _source.json：剔除 + warn（Q3）
// 路径是"删除未列出文件"能安全执行的前提，否则一个规则子源的目录可以成为
// 另一个规则子源目录的祖先，后者扫描未列出文件时会删掉前者的 marker
func TestValidateSourceFilesRules(t *testing.T) {
	var s SourceInfo
	raw := `{"source_id":"S","files":{"ok.toml":"1","sub/b.toml":"2","_source.json":"3","..":"4","a\\b.toml":"5"}}`
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		t.Fatal(err)
	}
	issues, rejected := validateSourceFiles(&s)
	if rejected {
		t.Fatal("结构问题不应导致整体拒绝")
	}
	if len(s.Files) != 1 || s.Files["ok.toml"] != "1" {
		t.Errorf("应只保留 ok.toml，实际 %v", s.Files)
	}
	if len(issues) != 4 {
		t.Errorf("issues = %d 条, want 4: %+v", len(issues), issues)
	}
}

// 逐文件比对：token 相同则不下载；文件缺失则退回待下载
func TestPerFileTokenDiff(t *testing.T) {
	rulesDir := t.TempDir()
	dest := filepath.Join(rulesDir, "src")
	if err := os.MkdirAll(dest, 0755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"a.toml": "A1", "b.toml": "B1", "c.toml": "C1"} {
		if err := os.WriteFile(filepath.Join(dest, name), []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	writeMarker(t, dest, "src", map[string]string{"a.toml": "1", "b.toml": "1", "c.toml": "1"})

	// 只有 b 的 token 变了
	l := leafSrc{
		id:      "src",
		destDir: "src",
		files:   map[string]string{"a.toml": "1", "b.toml": "2", "c.toml": "1"},
		need:    map[string]string{},
	}
	local := loadLocalFileTokens(dest)
	for name, token := range l.files {
		if local[name] != token {
			l.need[name] = token
		}
	}
	if len(l.need) != 1 || l.need["b.toml"] != "2" {
		t.Fatalf("need = %v, want 仅 b.toml", l.need)
	}
	// 三个文件都在盘上，补齐检查不应改变 need
	queueMissingFiles(rulesDir, &l)
	if len(l.need) != 1 {
		t.Errorf("queueMissingFiles 不应改动 need，实际 %v", l.need)
	}
}

// 上游删掉的文件不下载，交给不变量 B 删除
func TestPerFileDiffDoesNotReaddRemoved(t *testing.T) {
	rulesDir := t.TempDir()
	dest := filepath.Join(rulesDir, "src")
	if err := os.MkdirAll(dest, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, "gone.toml"), []byte("G"), 0644); err != nil {
		t.Fatal(err)
	}
	writeMarker(t, dest, "src", map[string]string{"gone.toml": "1", "keep.toml": "1"})
	local := loadLocalFileTokens(dest)

	l := leafSrc{
		id: "src", destDir: "src",
		files: map[string]string{"keep.toml": "2"},
		need:  map[string]string{},
	}
	for name, token := range l.files {
		if local[name] != token {
			l.need[name] = token
		}
	}
	queueMissingFiles(rulesDir, &l)
	// gone.toml 已不在 manifest 中，不该被重新下载（否则删了又拉回来）
	if _, ok := l.need["gone.toml"]; ok {
		t.Error("上游已删除的文件被重新加入待下载")
	}
	if len(l.need) != 1 || l.need["keep.toml"] != "2" {
		t.Errorf("need = %v, want 仅 keep.toml", l.need)
	}
}

// token 未变但文件已不在盘上：必须退回待下载，否则该规则文件永久缺失
// （下次同步的 token 比对会认为它已是最新）
func TestPerFileDiffRefetchesMissingFile(t *testing.T) {
	rulesDir := t.TempDir()
	dest := filepath.Join(rulesDir, "src")
	if err := os.MkdirAll(dest, 0755); err != nil {
		t.Fatal(err)
	}
	writeMarker(t, dest, "src", map[string]string{"a.toml": "1", "b.toml": "1"})
	if err := os.WriteFile(filepath.Join(dest, "a.toml"), []byte("A"), 0644); err != nil {
		t.Fatal(err)
	}
	local := loadLocalFileTokens(dest)

	l := leafSrc{
		id: "src", destDir: "src",
		files: map[string]string{"a.toml": "1", "b.toml": "1"},
		need:  map[string]string{},
	}
	for name, token := range l.files {
		if local[name] != token {
			l.need[name] = token
		}
	}
	if len(l.need) != 0 {
		t.Fatalf("token 全一致，need 应为空，实际 %v", l.need)
	}
	queueMissingFiles(rulesDir, &l)
	if l.need["b.toml"] != "1" {
		t.Errorf("缺失文件未被退回待下载: need = %v", l.need)
	}
	if len(l.need) != 1 {
		t.Errorf("只有 b 缺失，need 应只有 b，实际 %v", l.need)
	}
}

// marker 读不出来时返回 nil（无可信基线），后果仅是本轮多下文件
func TestLoadLocalFileTokensNoBaseline(t *testing.T) {
	if got := loadLocalFileTokens(t.TempDir()); got != nil {
		t.Errorf("目录不存在时应返回 nil，实际 %v", got)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, sourceFileName), []byte("{不是 json"), 0644); err != nil {
		t.Fatal(err)
	}
	if got := loadLocalFileTokens(dir); got != nil {
		t.Errorf("marker 损坏时应返回 nil，实际 %v", got)
	}
	// list 型 marker 没有规则 token 表
	if err := os.WriteFile(filepath.Join(dir, sourceFileName), []byte(`{"source_id":"s","type":"list","files":["a/_source.json"]}`), 0644); err != nil {
		t.Fatal(err)
	}
	if got := loadLocalFileTokens(dir); got != nil {
		t.Errorf("list 型 marker 应返回 nil，实际 %v", got)
	}
}
