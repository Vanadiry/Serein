package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// TempCheckPlatform 单平台检查结果
type TempCheckPlatform struct {
	CurrentVersion string `json:"current_version,omitempty"`
	LatestVersion  string `json:"latest_version"`
	URL            any    `json:"url,omitempty"`
}

// TempCheckResult 单次检查结果缓存
type TempCheckResult struct {
	AppID     string                       `json:"app_id"`
	Name      string                       `json:"name"`
	Platforms map[string]TempCheckPlatform `json:"platforms"`
}

// SaveCheckTemp 保存检查结果到 HOME/temp/check/{typ}/ 下，最多保留 10 个文件。
func SaveCheckTemp(home, typ string, results []TempCheckResult) error {
	dir := filepath.Join(home, "temp", "check", typ)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	name := time.Now().Format("20060102_150405") + ".json"
	path := filepath.Join(dir, name)
	data, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return err
	}

	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil
	}
	sort.Sort(sort.Reverse(sort.StringSlice(files)))
	if len(files) > 10 {
		for _, f := range files[10:] {
			os.Remove(f)
		}
	}
	return nil
}

// LoadLatestCheckTemp 加载对应类型最新的缓存文件。
func LoadLatestCheckTemp(home, typ string) ([]TempCheckResult, error) {
	dir := filepath.Join(home, "temp", "check", typ)
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil || len(files) == 0 {
		return nil, nil
	}
	sort.Sort(sort.Reverse(sort.StringSlice(files)))

	data, err := os.ReadFile(files[0])
	if err != nil {
		return nil, err
	}
	var results []TempCheckResult
	if err := json.Unmarshal(data, &results); err != nil {
		return nil, err
	}
	return results, nil
}
