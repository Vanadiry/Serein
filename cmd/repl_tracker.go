// REPL tracker 命令
package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

func handleTrackerCmd(home string, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: tracker <add|new|list> ...")
	}

	switch args[0] {
	case "add":
		return trackerAdd(home, args[1:])
	case "new":
		return trackerNew(home, args[1:])
	case "list":
		return trackerList(home, args[1:])
	default:
		return fmt.Errorf("unknown tracker subcommand: %s", args[0])
	}
}

func trackerAdd(home string, args []string) error {
	id := ""
	var platforms []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-id":
			i++
			if i < len(args) {
				id = args[i]
			}
		case "-platform":
			i++
			if i < len(args) && args[i] != "" {
				platforms = append(platforms, args[i])
			}
		default:
			if !strings.HasPrefix(args[i], "-") {
				id = args[i]
			}
		}
	}
	if id == "" {
		return fmt.Errorf("usage: tracker add <id> [-platform os]...")
	}
	body, _ := json.Marshal(map[string]any{
		"tracker_id": "_serein",
		"app_id":     id,
		"platforms":  platforms,
	})
	return apiPost(home, "/api/tracker/add", body)
}

func trackerNew(home string, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: tracker new <id>")
	}
	body, _ := json.Marshal(map[string]string{"id": args[0]})
	return apiPost(home, "/api/tracker/new", body)
}

func trackerList(home string, args []string) error {
	if len(args) > 0 {
		return apiGet(home, "/api/tracker/list/"+args[0])
	}
	return apiGet(home, "/api/tracker/list/all")
}

// ── HTTP helpers ──

func apiAddr(home string) string {
	// 从 config 读取端口
	cfg, _ := loadCfg(home)
	return fmt.Sprintf("http://%s:%d", cfg.Serein.Host, cfg.Serein.Port)
}

func apiGet(home, path string) error {
	resp, err := http.Get(apiAddr(home) + path)
	if err != nil {
		return fmt.Errorf("GET %s: %w", path, err)
	}
	defer resp.Body.Close()
	return printJSON(resp)
}

func apiPost(home, path string, body []byte) error {
	resp, err := http.Post(apiAddr(home)+path, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("POST %s: %w", path, err)
	}
	defer resp.Body.Close()
	return printJSON(resp)
}
