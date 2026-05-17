package store

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// SourceJSON 规则源的元信息
type SourceJSON struct {
	ID      string   `json:"id"`
	Mode    string   `json:"mode"`
	BaseURL string   `json:"baseurl,omitempty"`
	Files   []string `json:"files,omitempty"`
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
	var result SyncResult
	usedIDs := make(map[string]bool)

	for _, src := range sources {
		srcJSON, _, err := fetchSourceJSON(src.URL)
		if err != nil {
			result.Errors = append(result.Errors, SyncError{
				URL:    src.URL,
				Reason: fmt.Sprintf("获取 source.json 失败: %v", err),
			})
			continue
		}

		if usedIDs[srcJSON.ID] {
			result.Errors = append(result.Errors, SyncError{
				ID:     srcJSON.ID,
				URL:    src.URL,
				Reason: "ID 重复，已被先处理的规则源占用",
			})
			continue
		}
		usedIDs[srcJSON.ID] = true

		destDir := filepath.Join(home, "rule", srcJSON.ID)
		if err := syncSource(srcJSON, src.URL, destDir); err != nil {
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
	resp, err := http.Get(url)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}

	var s SourceJSON
	if err := json.Unmarshal(body, &s); err != nil {
		return nil, nil, fmt.Errorf("解析 source.json: %w", err)
	}
	if s.ID == "" {
		return nil, nil, fmt.Errorf("source.json 缺少 id")
	}
	return &s, body, nil
}

func syncSource(s *SourceJSON, sourceURL string, destDir string) error {
	switch s.Mode {
	case "github":
		return syncGitHub(sourceURL, destDir)
	case "web":
		return syncFileList(s.BaseURL, s.Files, destDir, true)
	case "local":
		return syncFileList(s.BaseURL, s.Files, destDir, false)
	default:
		return fmt.Errorf("未知 mode: %s", s.Mode)
	}
}

// ── GitHub ──

func syncGitHub(rawURL, destDir string) error {
	owner, repo, err := parseGitHubOwnerRepo(rawURL)
	if err != nil {
		return err
	}

	zipURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/zipball", owner, repo)
	resp, err := http.Get(zipURL)
	if err != nil {
		return fmt.Errorf("下载 zip: %w", err)
	}
	defer resp.Body.Close()

	zipBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取 zip: %w", err)
	}

	return extractZipFlat(zipBody, destDir)
}

func parseGitHubOwnerRepo(url string) (string, string, error) {
	// https://raw.githubusercontent.com/{owner}/{repo}/...
	prefix := "raw.githubusercontent.com/"
	idx := strings.Index(url, prefix)
	if idx < 0 {
		return "", "", fmt.Errorf("不是 raw.githubusercontent.com URL")
	}
	rest := url[idx+len(prefix):]
	parts := strings.SplitN(rest, "/", 3)
	if len(parts) < 2 {
		return "", "", fmt.Errorf("无法解析 owner/repo")
	}
	return parts[0], parts[1], nil
}

func extractZipFlat(zipBody []byte, destDir string) error {
	r, err := zip.NewReader(bytes.NewReader(zipBody), int64(len(zipBody)))
	if err != nil {
		return fmt.Errorf("解压 zip: %w", err)
	}

	os.RemoveAll(destDir)

	// GitHub zip 内第一层是 {repo}-{commit}/，去掉这一层
	var prefix string
	for _, f := range r.File {
		parts := strings.SplitN(f.Name, "/", 2)
		if len(parts) == 2 {
			prefix = parts[0] + "/"
			break
		}
	}

	for _, f := range r.File {
		relPath := strings.TrimPrefix(f.Name, prefix)
		if relPath == "" || strings.HasSuffix(relPath, "/") {
			continue
		}
		target := filepath.Join(destDir, relPath)
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.Create(target)
		if err != nil {
			rc.Close()
			return err
		}
		_, err = io.Copy(out, rc)
		rc.Close()
		out.Close()
		if err != nil {
			return err
		}
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
