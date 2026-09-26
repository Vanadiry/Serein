package store

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
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

// seedLeaf 造一个已提交过的子规则源：目标目录里有旧文件 + 旧版本标记
func seedLeaf(t *testing.T, rulesDir, destDir string, oldVersion int, oldFiles map[string]string) {
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
	raw := `{"source_id":"s","version":` + strconv.Itoa(oldVersion) + `,"files":[]}`
	if err := os.WriteFile(filepath.Join(dest, "_source.json"), []byte(raw), 0644); err != nil {
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

// #2 核心回归：提交成功后目标目录是新内容，版本标记一并换入，旧文件消失
func TestCommitLeafDirReplacesTree(t *testing.T) {
	home := t.TempDir()
	rulesDir := filepath.Join(home, "rules")
	staging, err := prepareStagingRoot(home)
	if err != nil {
		t.Fatal(err)
	}
	destDir := "src"
	seedLeaf(t, rulesDir, destDir, 1, map[string]string{
		"old.toml":      "OLD",
		"sub/old2.toml": "OLD2",
	})

	newRaw := `{"source_id":"src","version":2,"files":["a.toml"]}`
	err = commitLeafDir(staging, filepath.Join(rulesDir, destDir), map[string][]byte{
		"a.toml":     []byte("NEW"),
		"sub/b.toml": []byte("NEW2"),
	}, []byte(newRaw))
	if err != nil {
		t.Fatalf("commitLeafDir: %v", err)
	}

	dest := filepath.Join(rulesDir, destDir)
	if got := mustRead(t, filepath.Join(dest, "a.toml")); got != "NEW" {
		t.Errorf("a.toml = %q", got)
	}
	if got := mustRead(t, filepath.Join(dest, "sub", "b.toml")); got != "NEW2" {
		t.Errorf("sub/b.toml = %q", got)
	}
	if got := mustRead(t, filepath.Join(dest, "_source.json")); got != newRaw {
		t.Errorf("_source.json = %q", got)
	}
	if _, err := os.Stat(filepath.Join(dest, "old.toml")); !os.IsNotExist(err) {
		t.Error("旧文件未被清除")
	}
	if _, err := os.Stat(filepath.Join(dest, "sub", "old2.toml")); !os.IsNotExist(err) {
		t.Error("旧的嵌套文件未被清除")
	}
	if v := loadLocalSourceVersion(dest); v != 2 {
		t.Errorf("loadLocalSourceVersion = %d, want 2", v)
	}
}

// #2 核心回归：写文件失败必须放弃提交——目标目录保持原样，版本标记不推进
func TestCommitLeafDirKeepsOldTreeOnWriteFailure(t *testing.T) {
	home := t.TempDir()
	rulesDir := filepath.Join(home, "rules")
	staging, err := prepareStagingRoot(home)
	if err != nil {
		t.Fatal(err)
	}
	destDir := "src"
	seedLeaf(t, rulesDir, destDir, 1, map[string]string{"old.toml": "OLD"})

	// 暂存根目录置为不可写，使 leaf-xxx 目录创建失败 → 提交必然失败
	if err := os.Chmod(staging, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(staging, 0755) })

	err = commitLeafDir(staging, filepath.Join(rulesDir, destDir), map[string][]byte{
		"a.toml": []byte("NEW"),
	}, []byte(`{"source_id":"src","version":2}`))
	if err == nil {
		t.Fatal("预期提交失败（暂存目录不可写）")
	}

	dest := filepath.Join(rulesDir, destDir)
	if got := mustRead(t, filepath.Join(dest, "old.toml")); got != "OLD" {
		t.Errorf("失败后旧文件被破坏: %q", got)
	}
	if v := loadLocalSourceVersion(dest); v != 1 {
		t.Errorf("失败后版本标记被推进: %d, want 1（否则下次同步会永久跳过该源）", v)
	}
}

// #2：单个源提交失败要如实计入 sources_failed，另一个源照常提交
func TestCommitLeavesReportsFailure(t *testing.T) {
	home := t.TempDir()
	rulesDir := filepath.Join(home, "rules")
	staging, err := prepareStagingRoot(home)
	if err != nil {
		t.Fatal(err)
	}
	leaves := []leafSrc{
		{id: "ok", destDir: "ok", rawBody: []byte(`{"source_id":"ok","version":1}`)},
		{id: "bad", destDir: "bad", rawBody: []byte(`{"source_id":"bad","version":1}`)},
	}
	contents := []map[string][]byte{
		{"a.toml": []byte("A")},
		// a.toml 既是文件又是 b.toml 的父目录，无论写入顺序如何都必然失败
		{"a.toml": []byte("X"), "a.toml/b.toml": []byte("Y")},
	}
	leafFailed := []bool{false, false}

	updated, failures := commitLeaves(rulesDir, staging, leaves, contents, leafFailed)
	if updated != 1 {
		t.Errorf("updated = %d, want 1（bad 应失败）", updated)
	}
	if len(failures) != 1 || failures[0].Source != "bad" {
		t.Fatalf("failures = %+v, want 1 条且 source=bad", failures)
	}
	if got := mustRead(t, filepath.Join(rulesDir, "ok", "a.toml")); got != "A" {
		t.Errorf("ok 源未提交: %q", got)
	}
	// bad 源不该留下任何痕迹（连目录都不该建出来）
	if _, err := os.Stat(filepath.Join(rulesDir, "bad")); !os.IsNotExist(err) {
		t.Error("失败的源仍被创建出来")
	}
}

// 暂存目录不能落在 rules/ 内部，否则规则加载会把半成品 .toml 当规则读进来
func TestPrepareStagingRootIsOutsideRulesDir(t *testing.T) {
	home := t.TempDir()
	staging, err := prepareStagingRoot(home)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(staging) != home {
		t.Fatalf("暂存根应在 home 下，实际 %s", staging)
	}
	rel, _ := filepath.Rel(filepath.Join(home, "rules"), staging)
	if !strings.HasPrefix(rel, "..") {
		t.Fatalf("暂存根 %s 落在 rules/ 内部（rel=%s）", staging, rel)
	}
	if err := os.MkdirAll(filepath.Join(staging, "leaf-stale"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staging, "leaf-stale", "x.toml"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	staging2, err := prepareStagingRoot(home)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(staging2, "leaf-stale")); !os.IsNotExist(err) {
		t.Error("上次残留的暂存目录未被清理")
	}
}

// 提交成功后暂存根应为空（暂存目录与旧目录备份都已清理）
func TestCommitLeafDirLeavesNoStagingGarbage(t *testing.T) {
	home := t.TempDir()
	rulesDir := filepath.Join(home, "rules")
	staging, err := prepareStagingRoot(home)
	if err != nil {
		t.Fatal(err)
	}
	seedLeaf(t, rulesDir, "src", 1, map[string]string{"old.toml": "OLD"})
	for range 3 {
		if err := commitLeafDir(staging, filepath.Join(rulesDir, "src"), map[string][]byte{
			"a.toml": []byte("A"),
		}, []byte(`{"source_id":"src","version":2}`)); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := os.ReadDir(staging)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("暂存根残留: %v", names)
	}
}
