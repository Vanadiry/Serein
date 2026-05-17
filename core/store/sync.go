package store

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// SourceJSON 规则源的元信息（完整 _source.json 内容）
type SourceJSON struct {
	ID          string   `json:"source_id"`
	Name        string   `json:"name,omitempty"`
	Description string   `json:"description,omitempty"`
	Type        string   `json:"type,omitempty"`    // "rules"（默认）或 "list"
	Mode        string   `json:"mode,omitempty"`    // "web" 或 "local"，默认 web
	BaseURL     string   `json:"baseurl,omitempty"`
	Files       []string `json:"files"`
}

// SyncResult 同步结果
type SyncResult struct {
	Synced []string
	Errors []SyncError
}

// SyncError 同步错误
type SyncError struct {
	ID     string `json:"id,omitempty"`
	URL    string `json:"url"`
	Reason string `json:"reason"`
}

// SyncAllSources 同步所有规则源
func SyncAllSources(home string, sources []RuleSource) SyncResult {
	result := SyncResult{
		Synced: []string{},
		Errors: []SyncError{},
	}
	usedIDs := make(map[string]bool)

	for _, src := range sources {
		srcJSON, rawBody, err := fetchSourceJSON(src.URL)
		if err != nil {
			result.Errors = append(result.Errors, SyncError{
				URL:    src.URL,
				Reason: fmt.Sprintf("获取 _source.json 失败: %v", err),
			})
			continue
		}

		if len(srcJSON.Files) == 0 {
			result.Errors = append(result.Errors, SyncError{
				ID:     srcJSON.ID,
				URL:    src.URL,
				Reason: "_source.json 缺少 files 字段",
			})
			continue
		}

		if usedIDs[srcJSON.ID] {
			result.Errors = append(result.Errors, SyncError{
				ID:     srcJSON.ID,
				URL:    src.URL,
				Reason: "duplicate ID, already claimed by an earlier source",
			})
			continue
		}
		usedIDs[srcJSON.ID] = true

		destDir := filepath.Join(home, "rules", srcJSON.ID)
		if err := syncSource(srcJSON, rawBody, src.URL, destDir); err != nil {
			result.Errors = append(result.Errors, SyncError{
				ID:     srcJSON.ID,
				URL:    src.URL,
				Reason: fmt.Sprintf("同步失败: %v", err),
			})
			continue
		}
		result.Synced = append(result.Synced, srcJSON.ID)
	}
	return result
}

func fetchSourceJSON(url string) (*SourceJSON, []byte, error) {
	var body []byte
	var err error

	if strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") {
		resp, reqErr := http.Get(url)
		if reqErr != nil {
			return nil, nil, reqErr
		}
		defer resp.Body.Close()
		body, err = io.ReadAll(resp.Body)
	} else {
		body, err = os.ReadFile(url)
	}
	if err != nil {
		return nil, nil, err
	}

	var s SourceJSON
	if err := json.Unmarshal(body, &s); err != nil {
		return nil, nil, fmt.Errorf("解析 _source.json: %w", err)
	}
	if s.ID == "" {
		return nil, nil, fmt.Errorf("_source.json 缺少 source_id")
	}
	return &s, body, nil
}

func syncSource(s *SourceJSON, rawBody []byte, sourceURL string, destDir string) error {
	isLocal := s.Mode == "local"
	baseURL := s.BaseURL

	if baseURL == "" {
		baseURL = filepath.Dir(sourceURL)
		if !isLocal {
			idx := strings.LastIndex(sourceURL, "/")
			if idx >= 0 {
				baseURL = sourceURL[:idx]
			}
		}
	}

	if s.Type == "list" {
		return syncList(s, rawBody, baseURL, destDir, isLocal)
	}

	// type=rules（默认）：直接下载文件
	if err := syncFileList(baseURL, s.Files, destDir, !isLocal); err != nil {
		return err
	}

	infoPath := filepath.Join(destDir, "_source.json")
	if writeErr := os.WriteFile(infoPath, rawBody, 0644); writeErr != nil {
		return fmt.Errorf("write _source.json: %w", writeErr)
	}
	return nil
}

// syncList 递归处理 type=list 的源
func syncList(s *SourceJSON, rawBody []byte, baseURL string, destDir string, isLocal bool) error {
	os.RemoveAll(destDir)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}

	// 写入 list 源自身的 _source.json
	infoPath := filepath.Join(destDir, "_source.json")
	if err := os.WriteFile(infoPath, rawBody, 0644); err != nil {
		return err
	}

	for _, f := range s.Files {
		if filepath.Base(f) != "_source.json" {
			return fmt.Errorf("list 模式子源必须以 _source.json 结尾，实际为 %q", f)
		}
		subDir := filepath.Dir(f)
		expectedID := subDir

		// 获取子源
		var subURL string
		if isLocal {
			subURL = filepath.Join(baseURL, f)
		} else {
			subURL = strings.TrimSuffix(baseURL, "/") + "/" + f
		}

		subJSON, subRaw, err := fetchSourceJSON(subURL)
		if err != nil {
			return fmt.Errorf("子源 %s: %w", f, err)
		}

		if subJSON.ID != expectedID {
			return fmt.Errorf("子源 %s 的 source_id 必须为 %q，实际为 %q", f, expectedID, subJSON.ID)
		}

		subDest := filepath.Join(destDir, subDir)

		// 递归处理子源
		subBaseURL := baseURL
		if !isLocal {
			idx := strings.LastIndex(subURL, "/")
			if idx >= 0 {
				subBaseURL = subURL[:idx]
			}
		} else {
			subBaseURL = filepath.Dir(subURL)
		}

		if subJSON.Type == "list" {
			if err := syncList(subJSON, subRaw, subBaseURL, subDest, isLocal); err != nil {
				return err
			}
		} else {
			if err := syncFileList(subBaseURL, subJSON.Files, subDest, !isLocal); err != nil {
				return err
			}
			// 写入子源的 _source.json
			subInfoPath := filepath.Join(subDest, "_source.json")
			if writeErr := os.WriteFile(subInfoPath, subRaw, 0644); writeErr != nil {
				return fmt.Errorf("write _source.json: %w", writeErr)
			}
		}
	}
	return nil
}

// ── Web / Local 文件下载 ──

func syncFileList(baseURL string, files []string, destDir string, isWeb bool) error {
	os.RemoveAll(destDir)

	for _, f := range files {
		target := filepath.Join(destDir, f)
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}

		if isWeb {
			fileURL := strings.TrimSuffix(baseURL, "/") + "/" + f
			resp, err := http.Get(fileURL)
			if err != nil {
				return fmt.Errorf("下载 %s: %w", f, err)
			}
			defer resp.Body.Close()
			out, err := os.Create(target)
			if err != nil {
				return err
			}
			_, err = io.Copy(out, resp.Body)
			out.Close()
			if err != nil {
				return err
			}
		} else {
			src := filepath.Join(baseURL, f)
			if err := copyFile(src, target); err != nil {
				return fmt.Errorf("复制 %s: %w", f, err)
			}
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
