package store

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// TrackerEntry 单条追踪记录
type TrackerEntry struct {
	RuleID    string   `toml:"rule_id"`
	Platforms []string `toml:"platforms,omitempty"`
}

// trackerFile 一个 tracker 文件，可含多条 [[tracker]]
type trackerFile struct {
	Trackers []TrackerEntry `toml:"tracker"`
}

// LoadTracker 扫描 tracker/ 下所有 .toml，合并所有 [[tracker]] 条目。
func LoadTracker(home string) ([]TrackerEntry, error) {
	return loadTrackerFiles(home, "")
}

// LoadTrackerFile 加载指定 tracker 文件（按文件名，不含 .toml 后缀）。
func LoadTrackerFile(home, name string) ([]TrackerEntry, error) {
	return loadTrackerFiles(home, name)
}

func loadTrackerFiles(home, name string) ([]TrackerEntry, error) {
	dir := filepath.Join(home, "tracker")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read tracker dir: %w", err)
	}

	var list []TrackerEntry
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".toml") {
			continue
		}
		// 若指定了 name，只加载匹配文件
		if name != "" && strings.TrimSuffix(e.Name(), ".toml") != name {
			continue
		}
		path := filepath.Join(dir, e.Name())
		var tf trackerFile
		if err := decodeTOML(path, &tf); err != nil {
			continue
		}
		list = append(list, tf.Trackers...)
	}
	return list, nil
}

// TrackerExists 检查 tracker 文件（按文件名）是否存在。
func TrackerExists(home, name string) bool {
	path := filepath.Join(home, "tracker", name+".toml")
	_, err := os.Stat(path)
	return err == nil
}

// CreateTrackerFile 创建 tracker 文件，写入默认模板。
func CreateTrackerFile(home, name string) error {
	path := filepath.Join(home, "tracker", name+".toml")
	return os.WriteFile(path, []byte(DefaultTrackerTOML), 0644)
}

// FindTrackerEntry 在所有 tracker 文件中查找 rule_id。
func FindTrackerEntry(home, ruleID string) *TrackerEntry {
	list, _ := LoadTracker(home)
	for i := range list {
		if list[i].RuleID == ruleID {
			return &list[i]
		}
	}
	return nil
}

// PlatformsFor 返回 tracker 条目实际生效的平台列表。
func PlatformsFor(entry TrackerEntry, cfgPlatforms []string) []string {
	if len(entry.Platforms) > 0 {
		return entry.Platforms
	}
	return cfgPlatforms
}
