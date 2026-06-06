package main

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/vanadiry/serein/server"
)

//go:embed web
var webFiles embed.FS

// openBrowser 打开浏览器
func openBrowser(url string) {
	var c *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		c = exec.Command("open", url)
	case "windows":
		c = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		c = exec.Command("xdg-open", url)
	}
	_ = c.Start()
}

func main() {
	home := sereinHome()

	if err := initDirs(home); err != nil {
		fmt.Fprintf(os.Stderr, "Serein: %v\n", err)
		os.Exit(1)
	}

	// 启动 Web 服务 + 打开浏览器
	startServer(home, true)
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

func initDirs(home string) error {
	dirs := []string{
		home,
		filepath.Join(home, "rules"),
		filepath.Join(home, "tracker"),
		filepath.Join(home, "user"),
		filepath.Join(home, "temp", "check"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return fmt.Errorf("mkdir %s: %w", d, err)
		}
	}
	return nil
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
			openBrowser(addr)
		}()
	}
	if err := s.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Serein: %v\n", err)
		os.Exit(1)
	}
}
