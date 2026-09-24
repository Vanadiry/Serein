package store

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
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
	Order       int    `json:"order"`
	Type        string `json:"type"`
	Count       int    `json:"count"` // 条目数
}

// trackerFile 一个 tracker 文件（app 类型），可含多条 [[tracker]]
type trackerFile struct {
	DisplayName string         `toml:"display_name,omitempty"`
	Order       int            `toml:"order,omitempty"`
	Type        string         `toml:"type,omitempty"`
	Trackers    []TrackerEntry `toml:"tracker"`
}

// trackerMeta 顶层元信息（与类型无关）
type trackerMeta struct {
	DisplayName string `toml:"display_name,omitempty"`
	Order       int    `toml:"order,omitempty"`
	Type        string `toml:"type,omitempty"`
}

// decodeTrackerFile 读取 tracker 文件，返回元信息与条目
// msvsix / openvsx 使用单表 [tracker] app_id = [...]；其余使用 [[tracker]]
func decodeTrackerFile(path string) (trackerMeta, []TrackerEntry, error) {
	var meta trackerMeta
	if err := decodeTOML(path, &meta); err != nil {
		return meta, nil, err
	}

	if meta.Type == "msvsix" || meta.Type == "openvsx" {
		var v struct {
			Tracker struct {
				AppID []string `toml:"app_id"`
			} `toml:"tracker"`
		}
		if err := decodeTOML(path, &v); err != nil {
			return meta, nil, fmt.Errorf("%s：VSIX Tracker 需使用 [tracker] app_id = [...] 格式", filepath.Base(path))
		}
		entries := make([]TrackerEntry, 0, len(v.Tracker.AppID))
		for _, id := range v.Tracker.AppID {
			if id != "" {
				entries = append(entries, TrackerEntry{AppID: id})
			}
		}
		return meta, entries, nil
	}

	var a struct {
		Trackers []TrackerEntry `toml:"tracker"`
	}
	if err := decodeTOML(path, &a); err != nil {
		return meta, nil, err
	}
	return meta, a.Trackers, nil
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
		meta, trackerEntries, err := decodeTrackerFile(path)
		if err != nil {
			continue
		}
		name := meta.DisplayName
		if name == "" {
			name = id
		}
		trackerType := meta.Type
		if trackerType == "" {
			trackerType = "app"
		}
		list = append(list, TrackerInfo{ID: id, DisplayName: name, Order: meta.Order, Type: trackerType, Count: len(trackerEntries)})
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].Order != list[j].Order {
			return list[i].Order < list[j].Order
		}
		return list[i].DisplayName < list[j].DisplayName
	})
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
		_, trackerEntries, err := decodeTrackerFile(path)
		if err != nil {
			continue
		}
		list = append(list, trackerEntries...)
	}
	return list, nil
}

// ValidTrackerName 校验 tracker 名称是否安全
func ValidTrackerName(name string) bool {
	if name == "" || name == "." || name == ".." || filepath.IsAbs(name) {
		return false
	}
	return !strings.ContainsAny(name, `/\`)
}

// trackerPath 返回 home/tracker/<name>.toml，并确保结果不逃出 tracker 目录
func trackerPath(home, name string) (string, error) {
	dir := filepath.Join(home, "tracker")
	p := filepath.Join(dir, name+".toml")
	rel, err := filepath.Rel(dir, p)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid tracker name %q", name)
	}
	return p, nil
}

// GetTrackerType 返回 tracker 文件的 type（默认 "app"）。
func GetTrackerType(home, name string) string {
	path, err := trackerPath(home, name)
	if err != nil {
		return "app"
	}
	var meta trackerMeta
	if err := decodeTOML(path, &meta); err != nil {
		return "app"
	}
	if meta.Type == "" {
		return "app"
	}
	return meta.Type
}

// TrackerExists 检查 tracker 文件（按文件名）是否存在。
func TrackerExists(home, name string) bool {
	path, err := trackerPath(home, name)
	if err != nil {
		return false
	}
	_, statErr := os.Stat(path)
	return statErr == nil
}

// CreateTrackerFile 创建 tracker 文件，写入默认模板。
func CreateTrackerFile(home, name string) error {
	path, err := trackerPath(home, name)
	if err != nil {
		return err
	}
	return os.WriteFile(path, []byte(DefaultTrackerTOML), 0644)
}

// AddToTracker 追加 [[tracker]] 条目到指定 tracker 文件，保留 display_name。
// 若 app_id 已存在则合并平台（去重追加），否则新增条目。
func AddToTracker(home, name string, entry TrackerEntry) error {
	path, err := trackerPath(home, name)
	if err != nil {
		return err
	}

	var tf trackerFile
	if _, err := os.Stat(path); err == nil {
		if decErr := decodeTOML(path, &tf); decErr != nil {
			return fmt.Errorf("解析 tracker 文件失败 %s: %w", path, decErr)
		}
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

// PlatformsFor 返回 tracker 条目实际生效的平台列表。
func PlatformsFor(entry TrackerEntry, cfgPlatforms []string) []string {
	if len(entry.Platforms) > 0 {
		return entry.Platforms
	}
	return cfgPlatforms
}
