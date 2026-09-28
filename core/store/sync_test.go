package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSafeRelPath(t *testing.T) {
	base := "/tmp/base"
	cases := map[string]bool{
		"a/b.toml":         true,
		"sub/_source.json": true,
		"a.toml":           true,
		"../x":             false,
		"a/../../x":        false,
		"/abs":             false,
		"":                 false,
		"./":               false,
	}
	for rel, want := range cases {
		if _, got := safeRelPath(base, rel); got != want {
			t.Errorf("safeRelPath(%q) = %v, want %v", rel, got, want)
		}
	}
}

// seedLeaf 造一个已提交过的规则子源：目标目录里有旧文件 + 旧 token 标记
// oldTokens 显式给出上次接受的 token（缺省用文件名当 token）
func seedLeaf(t *testing.T, rulesDir, destDir string, oldTokens map[string]string, oldFiles map[string]string) {
	t.Helper()
	dest := filepath.Join(rulesDir, destDir)
	if err := os.MkdirAll(dest, 0755); err != nil {
		t.Fatal(err)
	}
	for rel, body := range oldFiles {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dest, rel)), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dest, rel), []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	tokens := make(map[string]string, len(oldFiles))
	for name := range oldFiles {
		if v, ok := oldTokens[name]; ok {
			tokens[name] = v
		} else {
			tokens[name] = "1"
		}
	}
	writeMarker(t, filepath.Join(dest), "s", tokens)
}

// writeMarker 写一个 rules 型源的 marker（token 表即“上次接受什么”的基线）
func writeMarker(t *testing.T, dir, id string, tokens map[string]string) {
	t.Helper()
	body, err := json.Marshal(SourceInfo{ID: id, Type: "rules", Files: tokens})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, sourceFileName), body, 0644); err != nil {
		t.Fatal(err)
	}
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取 %s 失败: %v", path, err)
	}
	return string(b)
}

// 核心回归：提交后新内容就位、marker 一并写入、旧文件与未列出的子目录被清除
func TestCommitLeafInPlaceWritesAndPrunes(t *testing.T) {
	home := t.TempDir()
	rulesDir := filepath.Join(home, "rules")
	destDir := "src"
	seedLeaf(t, rulesDir, destDir, nil, map[string]string{
		"old.toml":  "OLD",
		"keep.toml": "KEEP",
	})
	// 一个无 marker 的子目录：门禁 2 要求不得被动
	if err := os.MkdirAll(filepath.Join(rulesDir, destDir, "usercode"), 0755); err != nil {
		t.Fatal(err)
	}

	manifest := map[string]string{"keep.toml": "2", "new.toml": "2"}
	if err := commitLeafInPlace(filepath.Join(rulesDir, destDir), map[string][]byte{
		"keep.toml": []byte("KEEP2"),
		"new.toml":  []byte("NEW"),
	}, manifest, []byte(`{"source_id":"src","files":{"keep.toml":"2","new.toml":"2"}}`)); err != nil {
		t.Fatalf("commitLeafInPlace: %v", err)
	}

	dest := filepath.Join(rulesDir, destDir)
	if got := mustRead(t, filepath.Join(dest, "keep.toml")); got != "KEEP2" {
		t.Errorf("keep.toml = %q", got)
	}
	if got := mustRead(t, filepath.Join(dest, "new.toml")); got != "NEW" {
		t.Errorf("new.toml = %q", got)
	}
	// 不变量 B：manifest 未列出的 .toml 被删除
	if _, err := os.Stat(filepath.Join(dest, "old.toml")); !os.IsNotExist(err) {
		t.Error("未列出的旧文件未被清除")
	}
	// 门禁 2：无 marker 的子目录不得被动
	if _, err := os.Stat(filepath.Join(dest, "usercode")); err != nil {
		t.Errorf("无 marker 的子目录被误删: %v", err)
	}
	if got := loadLocalFileTokens(dest); got["new.toml"] != "2" || len(got) != 2 {
		t.Errorf("marker token 表 = %v", got)
	}
}

// 核心回归：写文件失败必须放弃本源剩余步骤，版本标记绝不推进
// 原地写拿不到“整树换入”那种“旧内容原样保留”的更强保证，文件可能已部分更新
// 但“标记不推进”这条必须保住，推进了就永久缺失
func TestCommitLeafInPlaceKeepsMarkerOnWriteFailure(t *testing.T) {
	home := t.TempDir()
	rulesDir := filepath.Join(home, "rules")
	destDir := "src"
	seedLeaf(t, rulesDir, destDir, nil, map[string]string{"old.toml": "OLD"})

	dest := filepath.Join(rulesDir, destDir)
	// 造一个“文件”形态的目录，使写入它必然失败
	if err := os.MkdirAll(filepath.Join(dest, "blocker"), 0755); err != nil {
		t.Fatal(err)
	}
	manifest := map[string]string{"old.toml": "2", "blocker": "2"}
	err := commitLeafInPlace(dest, map[string][]byte{
		"old.toml": []byte("OLD2"),
		"blocker":  []byte("X"),
	}, manifest, []byte(`{"source_id":"src","files":{"old.toml":"2","blocker":"2"}}`))
	if err == nil {
		t.Fatal("预期提交失败（blocker 是目录，写不进去）")
	}
	// 旧 marker 记录的是 old.toml=1，失败后必须仍是它（未被推进到 2）
	if got := loadLocalFileTokens(dest); got["old.toml"] != "1" {
		t.Errorf("失败后版本标记被推进: %v（下次同步会永久跳过该源）", got)
	}
	if _, ok := loadLocalFileTokens(dest)["blocker"]; ok {
		t.Error("失败后 marker 竟含未写入成功的 blocker")
	}
}

// #2：单个源提交失败要如实计入 sources_failed，另一个源照常提交
func TestCommitLeavesReportsFailure(t *testing.T) {
	home := t.TempDir()
	rulesDir := filepath.Join(home, "rules")
	leaves := []leafSrc{
		{id: "ok", destDir: "ok", files: map[string]string{"a.toml": "1"},
			rawBody: []byte(`{"source_id":"ok","files":{"a.toml":"1"}}`)},
		{id: "bad", destDir: "bad", files: map[string]string{"a.toml": "1", "a.toml/b.toml": "1"},
			rawBody: []byte(`{"source_id":"bad","files":{"a.toml":"1","a.toml/b.toml":"1"}}`)},
	}
	contents := []map[string][]byte{
		{"a.toml": []byte("A")},
		// a.toml 既是文件又是 b.toml 的父目录，无论写入顺序如何都必然失败
		{"a.toml": []byte("X"), "a.toml/b.toml": []byte("Y")},
	}
	leafFailed := []bool{false, false}

	updated, failures := commitLeaves(rulesDir, leaves, contents, leafFailed)
	if updated != 1 {
		t.Errorf("updated = %d, want 1（bad 应失败）", updated)
	}
	if len(failures) != 1 || failures[0].Source != "bad" {
		t.Fatalf("failures = %+v, want 1 条且 source=bad", failures)
	}
	if got := mustRead(t, filepath.Join(rulesDir, "ok", "a.toml")); got != "A" {
		t.Errorf("ok 源未提交: %q", got)
	}
	// 失败的源不得留下任何已提交内容（marker 未写入 = 界面上不可见）
	if _, err := os.Stat(filepath.Join(rulesDir, "bad", sourceFileName)); !os.IsNotExist(err) {
		t.Error("失败的源仍被写入了版本标记")
	}
}
