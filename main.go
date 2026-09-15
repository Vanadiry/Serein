package main

import (
	"embed"
	"errors"
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
	webFS, err := fs.Sub(webFiles, "web")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Serein: %v\n", err)
		os.Exit(1)
	}
	s, err := server.New(home, webFS)
	if err != nil {
		var ve *store.ValidationError
		if errors.As(err, &ve) {
			// SEREIN_ERROR=<标题> 指定错误页标题，其余行作为正文
			fmt.Fprintln(os.Stderr, "SEREIN_ERROR=配置有误")
			for _, e := range ve.Errors {
				fmt.Fprintln(os.Stderr, e)
			}
		} else {
			fmt.Fprintf(os.Stderr, "Serein: %v\n", err)
		}
		os.Exit(1)
	}
	if err := s.Listen(); err != nil {
		fmt.Fprintf(os.Stderr, "Serein: %v\n", err)
		os.Exit(1)
	}
	if openBrowser_ {
		go server.OpenBrowser("http://" + s.Addr())
	}
	if err := s.Serve(); err != nil {
		fmt.Fprintf(os.Stderr, "Serein: %v\n", err)
		os.Exit(1)
	}
}
