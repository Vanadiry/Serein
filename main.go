package main

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/vanadiry/serein/cmd"
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

	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "check":
			id := ""
			if len(os.Args) > 2 {
				id = os.Args[2]
			}
			if err := cmd.RunCheck(home, id); err != nil {
				fmt.Fprintf(os.Stderr, "Serein: %v\n", err)
				os.Exit(1)
			}
			return
		case "list":
			if err := cmd.RunList(home); err != nil {
				fmt.Fprintf(os.Stderr, "Serein: %v\n", err)
				os.Exit(1)
			}
			return
		case "sync":
			if err := cmd.RunSync(home); err != nil {
				fmt.Fprintf(os.Stderr, "Serein: %v\n", err)
				os.Exit(1)
			}
			return
		case "version":
			id := ""
			ver := ""
			if len(os.Args) > 2 {
				id = os.Args[2]
			}
			if len(os.Args) > 3 {
				ver = os.Args[3]
			}
			if id == "" {
				fmt.Fprintf(os.Stderr, "Usage: serein version <app_id> [version]\n")
				os.Exit(1)
			}
			if err := cmd.RunVersion(home, id, ver); err != nil {
				fmt.Fprintf(os.Stderr, "Serein: %v\n", err)
				os.Exit(1)
			}
			return
		case "tracker":
			if len(os.Args) < 3 {
				fmt.Fprintf(os.Stderr, "Usage: serein tracker <id>\n")
				os.Exit(1)
			}
			if err := cmd.RunTracker(home, os.Args[2]); err != nil {
				fmt.Fprintf(os.Stderr, "Serein: %v\n", err)
				os.Exit(1)
			}
			return
		case "cli":
			if err := cmd.RunREPL(home); err != nil {
				fmt.Fprintf(os.Stderr, "Serein: %v\n", err)
				os.Exit(1)
			}
			return
		default:
			fmt.Fprintf(os.Stderr, "Serein: unknown command %q\n", os.Args[1])
			os.Exit(1)
		}
		return
	}

	// 无参数：启动 Web 服务 + 打开浏览器
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
