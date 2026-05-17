package store

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// TrackerEntry 单条追踪记录
type TrackerEntry struct {
	RuleID    string   `toml:"rule_id,omitempty"`
	Platforms []string `toml:"platforms,omitempty"`
}

// trackerFile 单个 tracker 文件的 TOML 结构（rule_id 来自文件名）
type trackerFile struct {
	Platforms []string `toml:"platforms,omitempty"`
}

// LoadTracker 扫描 tracker/ 目录下所有 .toml 文件，文件名即为 rule_id。
func LoadTracker(home string) ([]TrackerEntry, error) {
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
		ruleID := strings.TrimSuffix(e.Name(), ".toml")
		path := filepath.Join(dir, e.Name())

		var tf trackerFile
		if err := decodeTOML(path, &tf); err != nil {
			continue
		}
		list = append(list, TrackerEntry{
			RuleID:    ruleID,
			Platforms: tf.Platforms,
		})
	}
	return list, nil
}

// TrackerExists 检查 tracker 文件是否存在。
func TrackerExists(home, ruleID string) bool {
	path := filepath.Join(home, "tracker", ruleID+".toml")
	_, err := os.Stat(path)
	return err == nil
}

// CreateTracker 创建 tracker 文件（不设 platforms，继承 config）。
func CreateTracker(home, ruleID string, platforms []string) error {
	tf := trackerFile{Platforms: platforms}
	path := filepath.Join(home, "tracker", ruleID+".toml")
	return encodeTOML(path, tf)
}

// RemoveTracker 删除 tracker 文件。
func RemoveTracker(home, ruleID string) error {
	path := filepath.Join(home, "tracker", ruleID+".toml")
	return os.Remove(path)
}

// PlatformsFor 返回 tracker 条目实际生效的平台列表。
func PlatformsFor(entry TrackerEntry, cfgPlatforms []string) []string {
	if len(entry.Platforms) > 0 {
		return entry.Platforms
	}
	return cfgPlatforms
}
