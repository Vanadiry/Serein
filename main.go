package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

func main() {
	home := sereinHome()

	// 初始化目录结构
	if err := initDirs(home); err != nil {
		fmt.Fprintf(os.Stderr, "Serein: %v\n", err)
		os.Exit(1)
	}

	// 路由到 CLI 或 serve 模式
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "check":
			cliCheck(home)
			return
		case "serve":
			// TODO: 阶段五
		default:
			fmt.Fprintf(os.Stderr, "Serein: unknown command %q\n", os.Args[1])
			os.Exit(1)
		}
		return
	}

	// 默认：启动服务 + 打开浏览器
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
		filepath.Join(home, "rule", "builtin"),
		filepath.Join(home, "rule", "_auto"),
		filepath.Join(home, "data"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return fmt.Errorf("mkdir %s: %w", d, err)
		}
	}
	return nil
}

func cliCheck(home string) {
	// TODO: 阶段四
	fmt.Println("Serein: check not implemented yet")
}

func startServer(home string) {
	// TODO: 阶段五
	fmt.Println("Serein: server not implemented yet")
}

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
