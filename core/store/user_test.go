package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveLoadUserData(t *testing.T) {
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, "user"), 0755)

	ud := UserData{"a": {"macos": "1.0"}}
	if err := SaveUserData(home, ud); err != nil {
		t.Fatal(err)
	}
	got, err := LoadUserData(home)
	if err != nil {
		t.Fatal(err)
	}
	if got["a"]["macos"] != "1.0" {
		t.Fatalf("got=%+v", got)
	}

	// 覆盖写
	ud["a"]["macos"] = "2.0"
	if err := SaveUserData(home, ud); err != nil {
		t.Fatal(err)
	}
	got, _ = LoadUserData(home)
	if got["a"]["macos"] != "2.0" {
		t.Fatalf("覆盖写失败: %+v", got)
	}

	// 不应残留临时文件
	entries, _ := os.ReadDir(filepath.Join(home, "user"))
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp") {
			t.Fatalf("残留临时文件: %s", e.Name())
		}
	}
}
func writeSoftware(t *testing.T, home, content string) {
	t.Helper()
	dir := filepath.Join(home, "user")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "software.json"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func listUserDir(t *testing.T, home string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(home, "user"))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

func TestLoadUserDataMissing(t *testing.T) {
	ud, err := LoadUserData(t.TempDir())
	if err != nil {
		t.Fatalf("文件不存在不应报错: %v", err)
	}
	if ud == nil {
		t.Fatal("应返回可用的空 map，不能是 nil")
	}
}

// 整份损坏：必须留档 + 返回空数据，绝不能报错后返回 nil 让上层 panic
func TestLoadUserDataCorruptQuarantines(t *testing.T) {
	home := t.TempDir()
	writeSoftware(t, home, `{"a": {"windows": "1.0"}`) // 少一个括号

	ud, err := LoadUserData(home)
	if err == nil {
		t.Fatal("应报错")
	}
	if ud == nil {
		t.Fatal("出错时也必须返回可用的空 map")
	}
	if len(ud) != 0 {
		t.Errorf("应为空，实际 %d 条", len(ud))
	}
	if !strings.Contains(err.Error(), "留档") {
		t.Errorf("错误信息应说明已留档: %v", err)
	}

	// 原文件被移走，留下一份可手工找回的副本
	names := listUserDir(t, home)
	var backups []string
	for _, n := range names {
		if strings.HasPrefix(n, "software.json.corrupt-") {
			backups = append(backups, n)
		}
	}
	if len(backups) != 1 {
		t.Fatalf("应留档 1 份，实际目录内容: %v", names)
	}
	body, err := os.ReadFile(filepath.Join(home, "user", backups[0]))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "1.0") {
		t.Errorf("留档内容应保留原始数据: %s", body)
	}
	if _, err := os.Stat(filepath.Join(home, "user", "software.json")); !os.IsNotExist(err) {
		t.Errorf("原文件应已被移走")
	}
}

// 两次损坏：留档文件名不能互相覆盖
func TestLoadUserDataQuarantineNoClobber(t *testing.T) {
	home := t.TempDir()
	for i := 0; i < 2; i++ {
		writeSoftware(t, home, `{"a":`)
		if _, err := LoadUserData(home); err == nil {
			t.Fatal("应报错")
		}
	}
	n := 0
	for _, name := range listUserDir(t, home) {
		if strings.HasPrefix(name, "software.json.corrupt-") {
			n++
		}
	}
	if n != 2 {
		t.Errorf("两次损坏应留 2 份档，实际 %d", n)
	}
}

// 单个键坏了不能连累其余全部：这是最常见的手滑（手写了 "foo": "1.2.3"）
func TestLoadUserDataSkipsBadKeysOnly(t *testing.T) {
	home := t.TempDir()
	writeSoftware(t, home, `{
		"good1": {"windows": "1.0", "linux": "2.0"},
		"foo": "1.2.3",
		"good2": {"windows": "3.0"},
		"badver": {"windows": 123},
		"partly": {"windows": "4.0", "osx": true}
	}`)

	ud, err := LoadUserData(home)
	if err == nil {
		t.Error("应报告坏掉的条目")
	}
	if got := ud["good1"]["windows"]; got != "1.0" {
		t.Errorf("good1/windows 应保留, 实际 %q", got)
	}
	if got := ud["good1"]["linux"]; got != "2.0" {
		t.Errorf("good1/linux 应保留, 实际 %q", got)
	}
	if got := ud["good2"]["windows"]; got != "3.0" {
		t.Errorf("good2 应完整保留, 实际 %q", got)
	}
	if got := ud["partly"]["windows"]; got != "4.0" {
		t.Errorf("partly 的好键应保留, 实际 %q", got)
	}
	if _, ok := ud["partly"]["osx"]; ok {
		t.Error("partly 的坏键应被跳过")
	}
	if _, ok := ud["foo"]; ok {
		t.Error("非对象的条目应被跳过")
	}
	if _, ok := ud["badver"]; ok {
		t.Error("版本号不是字符串的条目应被跳过")
	}
	if len(ud) != 3 {
		t.Errorf("应保留 3 个 app, 实际 %d: %v", len(ud), ud)
	}
	// 坏文件不应被留档——它大部分是好的，就地继续用
	for _, name := range listUserDir(t, home) {
		if strings.HasPrefix(name, "software.json.corrupt-") {
			t.Errorf("个别键坏掉不该留档: %s", name)
		}
	}
}

func TestLoadUserDataEmpty(t *testing.T) {
	home := t.TempDir()
	writeSoftware(t, home, "")
	ud, err := LoadUserData(home)
	if err != nil || len(ud) != 0 || ud == nil {
		t.Errorf("空文件应视为无数据: ud=%v err=%v", ud, err)
	}
}

// 损坏后确认操作应能自愈：写入新文件后重新读取不再报错
func TestSaveAfterCorruptRecovers(t *testing.T) {
	home := t.TempDir()
	writeSoftware(t, home, `{"a":`)

	ud, err := LoadUserData(home)
	if err == nil {
		t.Fatal("应报错")
	}
	ud["b"] = map[string]string{"windows": "9.9"}
	if err := SaveUserData(home, ud); err != nil {
		t.Fatal(err)
	}
	got, err := LoadUserData(home)
	if err != nil {
		t.Fatalf("写入后应恢复正常: %v", err)
	}
	if got["b"]["windows"] != "9.9" {
		t.Errorf("新数据应生效: %v", got)
	}
}
