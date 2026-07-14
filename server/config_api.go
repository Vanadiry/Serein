package server

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/vanadiry/serein/core/store"
)

var (
	reGitHubURL = regexp.MustCompile(`github\.com/([^/]+/[^/]+)`)
	reRawGitHub = regexp.MustCompile(`raw\.githubusercontent\.com/([^/]+/[^/]+)`)
)

func formatSourceURL(url string) string {
	if strings.Contains(url, "Vanadiry/SereinRulesList") {
		return "Serein 官方源"
	}
	if m := reGitHubURL.FindStringSubmatch(url); m != nil {
		return m[1] + " (GitHub)"
	}
	if m := reRawGitHub.FindStringSubmatch(url); m != nil {
		return m[1] + " (GitHub Raw)"
	}
	return url
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := store.LoadConfig(s.home)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	kv := map[string]string{
		"监听地址": fmt.Sprintf("%s:%d", cfg.Serein.Host, cfg.Serein.Port),
		"并发数":  fmt.Sprintf("%d", cfg.Download.Concurrency),
		"默认平台": strings.Join(cfg.Tracker.Platforms, ", "),
	}

	dl := cfg.Download.Downloader
	switch {
	case dl == "":
		kv["下载器"] = "无"
	case dl == "ndm":
		kv["下载器"] = "Neat Download Manager（内建）"
	case strings.Contains(dl, "{url}"):
		kv["下载器"] = "自定义命令"
	default:
		kv["下载器"] = "无"
	}

	if len(cfg.RuleSources) > 0 {
		var labels []string
		for _, rs := range cfg.RuleSources {
			labels = append(labels, formatSourceURL(rs.URL))
		}
		kv["规则源"] = strings.Join(labels, "\n")
	} else {
		kv["规则源"] = "未配置"
	}

	if cfg.Profile.URL != "" {
		kv["动态配置源"] = formatSourceURL(cfg.Profile.URL)
	} else {
		kv["动态配置源"] = "未配置"
	}

	if cfg.Serein.FirstRun {
		kv["起始页"] = "由前端控制"
	} else {
		kv["起始页"] = "已永久关闭"
	}

	p, err := store.LoadProfile(s.home)
	profileData := map[string]any{}
	if err == nil {
		profileData["known_extensions"] = p.KnownExtensions
		profileData["version_prefixes"] = p.VersionPrefixes
		profileData["version_suffixes"] = p.VersionSuffixes
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"config":    kv,
		"profile":   profileData,
		"first_run": cfg.Serein.FirstRun,
	})
}
