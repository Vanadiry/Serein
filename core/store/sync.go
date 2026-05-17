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

// SourceJSON 规则源的元信息（完整 source.json 内容）
type SourceJSON struct {
	ID          string   `json:"source_id"`
	Name        string   `json:"name,omitempty"`
	Description string   `json:"description,omitempty"`
	Mode        string   `json:"mode,omitempty"`  // "web" 或 "local"，默认 web
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
				Reason: fmt.Sprintf("获取 source.json 失败: %v", err),
			})
			continue
		}

		if len(srcJSON.Files) == 0 {
			result.Errors = append(result.Errors, SyncError{
				ID:     srcJSON.ID,
				URL:    src.URL,
				Reason: "source.json 缺少 files 字段",
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
		return nil, nil, fmt.Errorf("解析 source.json: %w", err)
	}
	if s.ID == "" {
		return nil, nil, fmt.Errorf("source.json 缺少 source_id")
	}
	return &s, body, nil
}

func syncSource(s *SourceJSON, rawBody []byte, sourceURL string, destDir string) error {
	isLocal := s.Mode == "local"
	baseURL := s.BaseURL

	// 未指定 baseurl → 取 source.json 同级目录
	if baseURL == "" {
		baseURL = filepath.Dir(sourceURL)
		if !isLocal {
			// web 模式：去掉末尾文件名，保留 scheme + 路径
			idx := strings.LastIndex(sourceURL, "/")
			if idx >= 0 {
				baseURL = sourceURL[:idx]
			}
		}
	}

	if err := syncFileList(baseURL, s.Files, destDir, !isLocal); err != nil {
		return err
	}

	// 写入完整 source.json 到 info.json
	infoPath := filepath.Join(destDir, "info.json")
	if writeErr := os.WriteFile(infoPath, rawBody, 0644); writeErr != nil {
		return fmt.Errorf("write info.json: %w", writeErr)
	}
	return nil
}

// ── Web / Local ──

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
