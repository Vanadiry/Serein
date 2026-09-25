// HTTP 服务：路由注册、启动、静态文件
package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/vanadiry/serein/core/checker"
	"github.com/vanadiry/serein/core/events"
	"github.com/vanadiry/serein/core/httpx"
	"github.com/vanadiry/serein/core/log"
	"github.com/vanadiry/serein/core/store"
)

type Server struct {
	home       string
	config     store.Config
	mux        *http.ServeMux
	webFS      fs.FS
	ln         net.Listener
	actualAddr string

	rulesMu sync.RWMutex
	rules   map[string]store.Rule
	rulesFP string

	// 检查结果缓存：按 Tracker（或 direct 的 type）分桶，内层按 app_id；随进程释放
	resultsMu sync.RWMutex
	results   map[string]map[string]checker.CheckResponse

	// 下载代理签名密钥；进程级随机，重启即失效
	proxySecret []byte
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Logf("[http] %s %s", r.Method, r.URL.Path)
		next.ServeHTTP(w, r)
	})
}

func withCORS() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 用请求的 Host 作为允许来源，兼容 host=0.0.0.0
			w.Header().Set("Access-Control-Allow-Origin", "http://"+r.Host)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// sameOriginGuard CSRF
func sameOriginGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			switch r.Header.Get("Sec-Fetch-Site") {
			case "cross-site", "same-site":
				writeError(w, http.StatusForbidden, "cross-site request blocked")
				return
			}
			if o := r.Header.Get("Origin"); o != "" {
				if u, err := url.Parse(o); err != nil || u.Host != r.Host {
					writeError(w, http.StatusForbidden, "origin not allowed")
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) registerRoutes() {
	s.mux.HandleFunc("POST /api/open-url", s.handleOpenURL)
	s.mux.HandleFunc("POST /api/download", s.handleDownload)
	s.mux.HandleFunc("GET /api/file", s.handleFile)

	s.mux.HandleFunc("GET /api/tracker/list/all", s.handleTrackerListAll)
	s.mux.HandleFunc("GET /api/tracker/list/", s.handleTrackerListByID)
	s.mux.HandleFunc("POST /api/tracker/new", s.handleTrackerNew)
	s.mux.HandleFunc("POST /api/tracker/add", s.handleTrackerAdd)
	s.mux.HandleFunc("GET /api/tracker/apps", s.handleTrackerApps)

	s.mux.HandleFunc("POST /api/check", s.handleCheck)
	s.mux.HandleFunc("POST /api/check/confirm", s.handleCheckConfirm)
	s.mux.HandleFunc("GET /api/check/temp/{type}", s.handleCheckTemp)
	s.mux.HandleFunc("GET /api/progress/{task_id}", handleProgressSSE)
	s.mux.HandleFunc("POST /api/check/cancel/{task_id}", handleProgressCancel)
	s.mux.HandleFunc("GET /api/events", handleEvents)
	s.mux.HandleFunc("GET /api/config", s.handleConfig)
	s.mux.HandleFunc("POST /api/sync", s.handleSync)

	s.mux.HandleFunc("GET /api/rules", s.handleRules)
	s.mux.HandleFunc("POST /api/rules/check", s.handleRulesCheck)
	s.mux.HandleFunc("GET /api/search", s.handleSearch)

	dlDesc := parseDownloaderDesc(s.config.Download.Downloader)
	dlType := parseDownloaderType(s.config.Download.Downloader)
	if s.webFS != nil {
		fileServer := http.FileServer(http.FS(s.webFS))
		s.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			if !strings.Contains(r.URL.Path, ".") {
				htmlPath := strings.TrimSuffix(r.URL.Path, "/") + ".html"
				if _, err := fs.Stat(s.webFS, strings.TrimPrefix(htmlPath, "/")); err == nil {
					r.URL.Path = htmlPath
				}
			}
			if r.URL.Path == "/assets/app.min.js" {
				data, err := fs.ReadFile(s.webFS, "assets/app.min.js")
				if err == nil {
					w.Header().Set("Content-Type", "application/javascript")
					data = bytes.Replace(data, []byte(`"__DL__"`), []byte(dlDesc), 1)
					data = bytes.Replace(data, []byte(`"__DL_TYPE__"`), []byte(`"`+dlType+`"`), 1)
					w.Write(data)
					return
				}
			}
			if r.URL.Path == "/" || r.URL.Path == "/index.html" {
				data, err := fs.ReadFile(s.webFS, "index.html")
				if err == nil {
					firstRun := "true"
					if !s.config.Serein.FirstRunEnabled() {
						firstRun = "false"
					}
					w.Header().Set("Content-Type", "text/html; charset=utf-8")
					w.Write(bytes.Replace(data, []byte(`__FIRST_RUN__`), []byte(firstRun), 1))
					return
				}
			}
			fileServer.ServeHTTP(w, r)
		})
	}
}

// 下载器类型：单一判定来源
const (
	dlBrowser = "browser"
	dlNDM     = "ndm"
	dlCustom  = "custom"
	dlUnknown = "unknown"
)

func downloaderKindOf(dl string) string {
	dl = strings.TrimSpace(dl)
	switch {
	case dl == "" || dl == "browser":
		return dlBrowser
	case dl == "ndm":
		return dlNDM
	case strings.Contains(dl, "{url}"):
		return dlCustom
	default:
		return dlUnknown
	}
}

func parseDownloaderType(dl string) string {
	switch downloaderKindOf(dl) {
	case dlNDM:
		return "ndm"
	case dlCustom:
		return "custom"
	default: // browser / unknown 均按浏览器处理
		return "browser"
	}
}

func parseDownloaderDesc(dl string) string {
	switch downloaderKindOf(dl) {
	case dlNDM:
		return `"Neat Download Manager（内建）"`
	case dlCustom:
		return `"自定义命令"`
	case dlUnknown:
		return `"无（未识别）"`
	default:
		return `"浏览器"`
	}
}

func New(home string, webFS fs.FS) (*Server, error) {
	cfg, err := store.LoadConfig(home)
	if err != nil {
		return nil, err
	}
	if errs := cfg.Validate(); len(errs) > 0 {
		return nil, &store.ValidationError{Errors: errs}
	}
	if err := log.InitLogger(home); err != nil {
		return nil, err
	}
	proxy := cfg.ProxyURL()
	httpx.SetProxy(proxy)
	// 所有发往 api.github.com 的请求自动带 GithubToken
	var authTargets []httpx.AuthTarget
	if cfg.Access.GithubToken != "" {
		authTargets = append(authTargets, httpx.AuthTarget{Host: "api.github.com", Token: cfg.Access.GithubToken})
	}
	httpx.SetAuthTargets(authTargets)
	if p, err := store.LoadProfile(home); err == nil {
		checker.SetVersionPrefixes(p.VersionPrefixes)
		checker.SetVersionSuffixes(p.VersionSuffixes)
	}
	s := &Server{home: home, config: cfg, mux: http.NewServeMux(), webFS: webFS, rules: make(map[string]store.Rule), results: make(map[string]map[string]checker.CheckResponse)}
	secret, err := newProxySecret()
	if err != nil {
		return nil, err
	}
	s.proxySecret = secret
	s.reloadRules()
	s.registerRoutes()
	return s, nil
}

// loadRules 重新解析并缓存全部规则。report 为真时把解析发现的问题上报到事件总线
func (s *Server) loadRules(report bool) []store.RuleIssue {
	rules, issues, err := store.LoadRules(s.home)
	if err != nil {
		events.Emit("error", "[rules]", fmt.Sprintf("加载规则失败: %v", err))
		return issues
	}
	s.rulesMu.Lock()
	s.rules = rules
	s.rulesFP = store.RulesFingerprint(s.home)
	s.rulesMu.Unlock()
	if report && len(issues) > 0 {
		// 聚合成一条，避免一次性弹出大量 toast（前端可滚动查看全部）
		errs, warns := 0, 0
		lines := make([]string, 0, len(issues))
		for _, is := range issues {
			if is.Level == "error" {
				errs++
			} else {
				warns++
			}
			lines = append(lines, fmt.Sprintf("[%s] %s", is.Level, is.Message))
		}
		level := "warn"
		if errs > 0 {
			level = "error"
		}
		events.Emit(level, "[rules]", fmt.Sprintf("规则检查：%d 个错误、%d 个警告\n%s", errs, warns, strings.Join(lines, "\n")))
	}
	return issues
}

// reloadRules 启动/拉取规则后调用：重新解析并上报问题
func (s *Server) reloadRules() {
	s.loadRules(true)
}

// getRules 返回规则表。rules/ 目录有变化时自动重新解析，否则用缓存
func (s *Server) getRules() map[string]store.Rule {
	fp := store.RulesFingerprint(s.home)
	s.rulesMu.RLock()
	if s.rules != nil && fp == s.rulesFP {
		rules := s.rules
		s.rulesMu.RUnlock()
		return rules
	}
	s.rulesMu.RUnlock()

	// 变化则重新解析，但不在读路径上报问题
	s.loadRules(false)
	s.rulesMu.RLock()
	rules := s.rules
	s.rulesMu.RUnlock()
	return rules
}

func (s *Server) Addr() string {
	if s.actualAddr != "" {
		return s.actualAddr
	}
	return fmt.Sprintf("%s:%d", s.config.Serein.Host, s.config.Serein.Port)
}

// Listen 绑定端口，并向Tauri报告实际监听地址。
func (s *Server) Listen() error {
	addr := fmt.Sprintf("%s:%d", s.config.Serein.Host, s.config.Serein.Port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", addr, err)
	}
	s.ln = ln
	s.actualAddr = ln.Addr().String()
	fmt.Fprintf(os.Stdout, "SEREIN_ADDR=%s\n", s.actualAddr)
	return nil
}

// Serve 在已绑定的监听器上提供服务，直到收到退出信号
func (s *Server) Serve() error {
	if s.ln == nil {
		if err := s.Listen(); err != nil {
			return err
		}
	}
	srv := &http.Server{Handler: withCORS()(sameOriginGuard(loggingMiddleware(s.mux)))}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Logf("listening on %s", s.actualAddr)
		if err := srv.Serve(s.ln); err != nil && err != http.ErrServerClosed {
			log.LogfError("server: %v", err)
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func limitBody(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
}

// isLoopback 判断请求是否来自本机（127.0.0.0/8 或 ::1）
func isLoopback(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
