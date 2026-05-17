package store

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// TrackerEntry 单条追踪记录
type TrackerEntry struct {
	AppID     string   `toml:"app_id"`
	Platforms []string `toml:"platforms,omitempty"`
}

// TrackerInfo tracker 文件元信息（前端侧栏用）
type TrackerInfo struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
}

// trackerFile 一个 tracker 文件，可含多条 [[tracker]]
type trackerFile struct {
	DisplayName string         `toml:"display_name,omitempty"`
	Trackers    []TrackerEntry `toml:"tracker"`
}

// LoadAllTrackerInfo 扫描 tracker/ 下所有 .toml，返回文件元信息列表。
func LoadAllTrackerInfo(home string) ([]TrackerInfo, error) {
	dir := filepath.Join(home, "tracker")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read tracker dir: %w", err)
	}

	var list []TrackerInfo
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".toml") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".toml")
		path := filepath.Join(dir, e.Name())
		var tf trackerFile
		if err := decodeTOML(path, &tf); err != nil {
			continue
		}
		name := tf.DisplayName
		if name == "" {
			name = id
		}
		list = append(list, TrackerInfo{ID: id, DisplayName: name})
	}
	return list, nil
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

// AddToTracker 追加 [[tracker]] 条目到指定 tracker 文件，保留 display_name。
// 若 app_id 已存在则合并平台（去重追加），否则新增条目。
func AddToTracker(home, name string, entry TrackerEntry) error {
	path := filepath.Join(home, "tracker", name+".toml")

	var tf trackerFile
	if _, err := os.Stat(path); err == nil {
		_ = decodeTOML(path, &tf)
	}

	// 查找已存在的 app_id
	for i := range tf.Trackers {
		if tf.Trackers[i].AppID == entry.AppID {
			// 合并平台
			existing := tf.Trackers[i].Platforms
			for _, p := range entry.Platforms {
				found := false
				for _, ep := range existing {
					if ep == p {
						found = true
						break
					}
				}
				if !found {
					tf.Trackers[i].Platforms = append(tf.Trackers[i].Platforms, p)
				}
			}
			return encodeTOML(path, tf)
		}
	}

	tf.Trackers = append(tf.Trackers, entry)
	return encodeTOML(path, tf)
}

// FindTrackerEntry 在所有 tracker 文件中查找 app_id。
func FindTrackerEntry(home, ruleID string) *TrackerEntry {
	list, _ := LoadTracker(home)
	for i := range list {
		if list[i].AppID == ruleID {
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
