// HTTP 服务：路由注册、启动、静态文件
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/vanadiry/serein/core/store"
)

type Server struct {
	home   string
	config store.Config
	mux    *http.ServeMux
}

func New(home string) (*Server, error) {
	cfg, err := store.LoadConfig(home)
	if err != nil {
		return nil, err
	}
	s := &Server{home: home, config: cfg, mux: http.NewServeMux()}
	s.registerRoutes()
	return s, nil
}

func (s *Server) registerRoutes() {
	s.mux.HandleFunc("/api/tracker/list/all", s.handleTrackerListAll)
	s.mux.HandleFunc("/api/tracker/list/", s.handleTrackerListByID)
	s.mux.HandleFunc("/api/tracker/new", s.handleTrackerNew)
	s.mux.HandleFunc("/api/tracker/add", s.handleTrackerAdd)
}

func (s *Server) Run() error {
	addr := fmt.Sprintf("%s:%d", s.config.Host, s.config.Port)
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
