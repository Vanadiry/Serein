// HTTP 服务：路由注册、启动、静态文件
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/vanadiry/serein/core/store"
)

type Server struct {
	home   string
	config store.Config
	mux    *http.ServeMux
	webFS  fs.FS
}

func (s *Server) registerRoutes() {
	s.mux.HandleFunc("/api/tracker/list/all", s.handleTrackerListAll)
	s.mux.HandleFunc("/api/tracker/list/", s.handleTrackerListByID)
	s.mux.HandleFunc("/api/tracker/new", s.handleTrackerNew)
	s.mux.HandleFunc("/api/tracker/add", s.handleTrackerAdd)

	s.mux.HandleFunc("/api/check/all", s.handleCheckAll)
	s.mux.HandleFunc("/api/check/ids", s.handleCheckIDs)
	s.mux.HandleFunc("/api/check/tracker", s.handleCheckTracker)
	s.mux.HandleFunc("/api/check/confirm", s.handleCheckConfirm)
	s.mux.HandleFunc("GET /api/check/temp/{type}", s.handleCheckTemp)

	s.mux.HandleFunc("/api/rules/sync", s.handleRulesSync)
	s.mux.HandleFunc("/api/rules/list/all", s.handleRulesListAll)
	s.mux.HandleFunc("/api/rules/list/search", s.handleRulesListSearch)
	s.mux.HandleFunc("/api/rules/list/", s.handleRulesListBySource)

	if s.webFS != nil {
		fileServer := http.FileServer(http.FS(s.webFS))
		s.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			// 无后缀 → 尝试补 .html
			if !strings.Contains(r.URL.Path, ".") {
				htmlPath := strings.TrimSuffix(r.URL.Path, "/") + ".html"
				if _, err := fs.Stat(s.webFS, strings.TrimPrefix(htmlPath, "/")); err == nil {
					r.URL.Path = htmlPath
				}
			}
			fileServer.ServeHTTP(w, r)
		})
	}
}

func New(home string, webFS fs.FS) (*Server, error) {
	cfg, err := store.LoadConfig(home)
	if err != nil {
		return nil, err
	}
	s := &Server{home: home, config: cfg, mux: http.NewServeMux(), webFS: webFS}
	s.registerRoutes()
	return s, nil
}

func (s *Server) Addr() string {
	return fmt.Sprintf("%s:%d", s.config.Serein.Host, s.config.Serein.Port)
}

func (s *Server) Run() error {
	addr := fmt.Sprintf("%s:%d", s.config.Serein.Host, s.config.Serein.Port)
	srv := &http.Server{Addr: addr, Handler: s.mux}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		fmt.Printf("Serein → http://%s\n", addr)
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			fmt.Printf("server: %v\n", err)
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
