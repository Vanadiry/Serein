package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/vanadiry/serein/cmd"
)

// openBrowser 打开浏览器（阶段五使用）
func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
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
			if err := cmd.RunCheck(home); err != nil {
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
				fmt.Fprintf(os.Stderr, "用法: serein version <rule_id> [version]\n")
				os.Exit(1)
			}
			if err := cmd.RunVersion(home, id, ver); err != nil {
				fmt.Fprintf(os.Stderr, "Serein: %v\n", err)
				os.Exit(1)
			}
			return
		case "serve":
			startServer(home)
		default:
			fmt.Fprintf(os.Stderr, "Serein: unknown command %q\n", os.Args[1])
			os.Exit(1)
		}
		return
	}

	startServer(home)
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
		filepath.Join(home, "rule"),
		filepath.Join(home, "user"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return fmt.Errorf("mkdir %s: %w", d, err)
		}
	}
	return nil
}

func startServer(home string) {
	// TODO: 阶段五
	fmt.Println("Serein: server not implemented yet")
}

