package main

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/vanadiry/serein/core/store"
	"github.com/vanadiry/serein/server"
)

//go:embed web
var webFiles embed.FS

func main() {
	home := sereinHome()

	if err := store.Init(home); err != nil {
		fmt.Fprintf(os.Stderr, "Serein: %v\n", err)
		os.Exit(1)
	}

	startServer(home, os.Getenv("SEREIN_SIDECAR") != "1")
}

func sereinHome() string {
	if v := os.Getenv("SEREIN_HOME"); v != "" {
		return v
	}
	dir, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Serein: cannot find home dir: %v\n", err)
		os.Exit(1)
	}
	return filepath.Join(dir, ".vSoft", "Serein")
}

func startServer(home string, openBrowser_ bool) {
	webFS, _ := fs.Sub(webFiles, "web")
	s, err := server.New(home, webFS)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Serein: %v\n", err)
		os.Exit(1)
	}
	if openBrowser_ {
		go func() {
			addr := "http://" + s.Addr()
			server.OpenBrowser(addr)
		}()
	}
	if err := s.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Serein: %v\n", err)
		os.Exit(1)
	}
}
