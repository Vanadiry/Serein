package store

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// SourceJSON 规则源的元信息（完整 _source.json 内容）
type SourceJSON struct {
	ID          string   `json:"source_id"`
	Name        string   `json:"name,omitempty"`
	Description string   `json:"description,omitempty"`
	Type        string   `json:"type,omitempty"` // "rules"（默认）或 "list"
	BaseURL     string   `json:"baseurl,omitempty"`
	Version     int      `json:"version,omitempty"`
	Files       []string `json:"files"`
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
	if !regexp.MustCompile(`^[a-zA-Z0-9_-]+$`).MatchString(s.ID) {
		return nil, nil, fmt.Errorf("source_id %q 包含非法字符，仅允许大小写字母、数字、下划线和连字符", s.ID)
	}
	return &s, body, nil
}

func loadLocalSourceVersion(dir string) int {
	data, err := os.ReadFile(filepath.Join(dir, "_source.json"))
	if err != nil {
		return 0
	}
	var s SourceJSON
	if json.Unmarshal(data, &s) != nil {
		return 0
	}
	return s.Version
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

func downloadFile(url, dest string) error {
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("下载 %s: %w", url, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	return os.WriteFile(dest, body, 0644)
}

// ── 异步同步（带进度）──────────────────────────────────────────

type leafSrc struct {
	id      string
	rawBody []byte
	destDir string
	baseURL string
	files   []string
	isWeb   bool
	version int
}

// SyncAllSourcesAsync 两阶段同步：
// 1. 遍历源树，收集所有叶子源 → 发 list 事件
// 2. 并发下载所有规则文件 → 发 file 事件（done/total）
func SyncAllSourcesAsync(home string, sources []RuleSource, concurrency int, p *Progress) {
	defer p.Close()
	concurrency = ClampConcurrency(concurrency)

	rulesDir := filepath.Join(home, "rules")

	// Phase 1: 遍历
	leaves := gatherLeaves(sources, concurrency, p)
	if len(leaves) == 0 {
		return
	}

	sourcesTotal := len(leaves)

	// 过滤版本未变的叶子源
	var fresh []leafSrc
	var skipped []leafSrc
	for _, l := range leaves {
		if l.version > 0 && loadLocalSourceVersion(filepath.Join(rulesDir, l.destDir)) == l.version {
			p.SendMap(map[string]any{
				"step":  "skip",
				"name":  l.id,
				"files": len(l.files),
			})
			skipped = append(skipped, l)
			continue
		}
		fresh = append(fresh, l)
	}
	leaves = fresh
	sourcesSkipped := len(skipped)
	sourcesUpdated := len(leaves)

	totalFiles := 0
	for _, l := range leaves {
		totalFiles += len(l.files)
	}

	if totalFiles > 0 {
		for _, l := range leaves {
			p.SendMap(map[string]any{
				"step":  "source",
				"name":  l.id,
				"files": len(l.files),
			})
		}
		p.Send("start", "", 0, totalFiles)
	}

	fileErrors := 0

	if totalFiles > 0 {
		sem := make(chan struct{}, concurrency)
		var mu sync.Mutex
		var done int
		var wg sync.WaitGroup

		for _, l := range leaves {
			dest := filepath.Join(rulesDir, l.destDir)
			if err := os.RemoveAll(dest); err != nil {
				Emit("error", "[sync]", fmt.Sprintf("清理目录失败 %s: %v", dest, err))
			}
			for _, f := range l.files {
				wg.Add(1)
				go func(l leafSrc, f string) {
					defer wg.Done()
					sem <- struct{}{}
					defer func() { <-sem }()

					target := filepath.Join(rulesDir, l.destDir, f)
					if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
						Emit("error", "[sync]", fmt.Sprintf("创建目录失败 %s: %v", filepath.Dir(target), err))
					}
					var err error
					if l.isWeb {
						err = downloadFile(strings.TrimSuffix(l.baseURL, "/")+"/"+f, target)
					} else {
						err = copyFile(filepath.Join(l.baseURL, f), target)
					}
					mu.Lock()
					done++
					name := f
					if err != nil {
						name += " (失败)"
						fileErrors++
					}
					p.Send("file", name, done, totalFiles)
					mu.Unlock()
				}(l, f)
			}
		}
		wg.Wait()

		// 写入 _source.json
		for _, l := range leaves {
			dest := filepath.Join(rulesDir, l.destDir)
			os.MkdirAll(dest, 0755)
			if err := os.WriteFile(filepath.Join(dest, "_source.json"), l.rawBody, 0644); err != nil {
				Emit("error", "[sync]", fmt.Sprintf("写入 _source.json 失败 %s: %v", l.destDir, err))
			}
		}
	}

	// Phase 3: 清理孤立文件（所有源：已更新 + 已跳过）
	var allLeaves []leafSrc
	allLeaves = append(allLeaves, leaves...)
	allLeaves = append(allLeaves, skipped...)

	deletedDir := filepath.Join(rulesDir, "_deleted")
	os.MkdirAll(deletedDir, 0755)
	var deletedFiles int

	for _, l := range allLeaves {
		fileSet := make(map[string]bool)
		for _, f := range l.files {
			fileSet[f] = true
		}
		dest := filepath.Join(rulesDir, l.destDir)
		entries, err := os.ReadDir(dest)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || e.Name() == "_source.json" {
				continue
			}
			if !fileSet[e.Name()] {
				targetDir := filepath.Join(deletedDir, l.destDir)
				os.MkdirAll(targetDir, 0755)
				oldPath := filepath.Join(dest, e.Name())
				newPath := filepath.Join(targetDir, e.Name())
				os.Remove(newPath)
				if err := os.Rename(oldPath, newPath); err != nil {
					Emit("error", "[sync]", fmt.Sprintf("移动孤立文件失败 %s: %v", e.Name(), err))
				} else {
					deletedFiles++
				}
			}
		}
	}

	p.SendMap(map[string]any{
		"step":            "done",
		"sources_total":   sourcesTotal,
		"sources_skipped": sourcesSkipped,
		"sources_updated": sourcesUpdated,
		"files":           totalFiles,
		"file_errors":     fileErrors,
		"deleted_files":   deletedFiles,
	})
}

func gatherLeaves(sources []RuleSource, concurrency int, p *Progress) []leafSrc {
	sem := make(chan struct{}, concurrency)
	var mu sync.Mutex
	var wg sync.WaitGroup
	usedIDs := make(map[string]bool)
	var leaves []leafSrc

	for _, src := range sources {
		wg.Add(1)
		go func(src RuleSource) {
			defer wg.Done()
			sem <- struct{}{}
			js, raw, err := fetchSourceJSON(src.URL)
			<-sem
			if err != nil {
				p.Send("error", src.URL+" 获取失败", 0, 0)
				return
			}
			mu.Lock()
			if usedIDs[js.ID] {
				mu.Unlock()
				return
			}
			usedIDs[js.ID] = true
			mu.Unlock()

			p.Send("list", js.ID, 0, 0)
			sub := resolveLeaves(js, raw, src.URL, js.ID, usedIDs, sem, &mu, p)
			mu.Lock()
			leaves = append(leaves, sub...)
			mu.Unlock()
		}(src)
	}
	wg.Wait()
	return leaves
}

func resolveLeaves(s *SourceJSON, rawBody []byte, sourceURL, destRel string, usedIDs map[string]bool, sem chan struct{}, mu *sync.Mutex, p *Progress) []leafSrc {
	isLocal := !strings.HasPrefix(sourceURL, "http://") && !strings.HasPrefix(sourceURL, "https://")
	baseURL := s.BaseURL
	if baseURL == "" {
		baseURL = filepath.Dir(sourceURL)
		if !isLocal {
			if idx := strings.LastIndex(sourceURL, "/"); idx >= 0 {
				baseURL = sourceURL[:idx]
			}
		}
	}
	if s.Type != "list" {
		return []leafSrc{{id: s.ID, rawBody: rawBody, destDir: destRel, baseURL: baseURL, files: s.Files, isWeb: !isLocal, version: s.Version}}
	}
	var result []leafSrc
	for _, f := range s.Files {
		if filepath.Base(f) != "_source.json" {
			continue
		}
		subDir := filepath.Dir(f)
		var subURL string
		if isLocal {
			subURL = filepath.Join(baseURL, f)
		} else {
			subURL = strings.TrimSuffix(baseURL, "/") + "/" + f
		}
		sem <- struct{}{}
		subJSON, subRaw, err := fetchSourceJSON(subURL)
		<-sem
		if err != nil {
			p.Send("error", subURL+" 获取失败", 0, 0)
			continue
		}
		if subJSON.ID != subDir {
			continue
		}
		mu.Lock()
		if usedIDs[subJSON.ID] {
			mu.Unlock()
			continue
		}
		usedIDs[subJSON.ID] = true
		mu.Unlock()
		result = append(result, resolveLeaves(subJSON, subRaw, subURL, filepath.Join(destRel, subDir), usedIDs, sem, mu, p)...)
	}
	return result
}
