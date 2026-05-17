// REPL check
package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/vanadiry/serein/core/store"
)

func handleCheckCmd(home string, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: check <all|app|tracker|confirm> ...")
	}

	// 解析 -auto 和 -temp
	auto := false
	temp := false
	var filtered []string
	for _, a := range args {
		if a == "-auto" {
			auto = true
		} else if a == "-temp" {
			temp = true
		} else {
			filtered = append(filtered, a)
		}
	}

	if temp {
		return checkTemp(home, filtered)
	}
	if auto {
		return checkAuto(home, filtered)
	}

	// 非 AUTO 模式
	switch filtered[0] {
	case "confirm":
		return checkConfirm(home, filtered[1:])
	default:
		return checkNormal(home, filtered)
	}
}

// ── 普通检查 ──

func checkNormal(home string, args []string) error {
	switch args[0] {
	case "all":
		return apiPost(home, "/api/check/all", nil)
	case "app":
		if len(args) < 2 {
			return fmt.Errorf("usage: check app <id...>")
		}
		body, _ := json.Marshal(args[1:])
		return apiPost(home, "/api/check/ids", body)
	case "tracker":
		if len(args) < 2 {
			return fmt.Errorf("usage: check tracker <id>")
		}
		body, _ := json.Marshal(map[string]string{"tracker_id": args[1]})
		return apiPost(home, "/api/check/tracker", body)
	default:
		return fmt.Errorf("unknown: check %s", args[0])
	}
}

// ── check confirm ──

func checkConfirm(home string, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: check confirm <id> <version> or <id> <os:ver>...")
	}
	body := map[string]string{"app_id": args[0]}
	if len(args) == 2 && !strings.Contains(args[1], ":") {
		// 单版本 → 所有平台
		body["macos"] = args[1]
		body["windows"] = args[1]
		body["linux"] = args[1]
	} else {
		for _, a := range args[1:] {
			parts := strings.SplitN(a, ":", 2)
			if len(parts) == 2 {
				body[parts[0]] = parts[1]
			}
		}
	}
	b, _ := json.Marshal(body)
	return apiPost(home, "/api/check/confirm", b)
}

// ── check -temp ──

func checkTemp(home string, args []string) error {
	typ := "all"
	if len(args) > 1 {
		typ = args[1]
	}
	return apiGet(home, "/api/check/temp/"+typ)
}

// ── AUTO 模式 ──

type autoItem struct {
	AppID     string
	Name      string
	Platforms []autoPlatform
}
type autoPlatform struct {
	OS      string
	Current string
	Latest  string
	URL     any
}

func checkAuto(home string, args []string) error {
	// 发起检查
	var body []byte
	switch args[0] {
	case "all":
		body = nil
	case "app":
		b, _ := json.Marshal(args[1:])
		body = b
	case "tracker":
		b, _ := json.Marshal(map[string]string{"tracker_id": args[1]})
		body = b
	default:
		return fmt.Errorf("unknown: check %s -auto", args[0])
	}

	path := "/api/check/" + args[0]
	if args[0] == "app" {
		path = "/api/check/ids"
	}
	resp, err := http.Post(apiAddr(home)+path, "application/json", strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	io.ReadAll(resp.Body)
	resp.Body.Close()

	// 从缓存读取（含 current_version）
	typ := args[0]
	if typ == "app" {
		typ = "ids"
	}
	var results []store.TempCheckResult
	raw, _ := apiGetRaw(home, "/api/check/temp/"+typ)
	json.Unmarshal(raw, &results)

	// 过滤有更新的
	var items []autoItem
	for _, r := range results {
		item := autoItem{AppID: r.AppID, Name: r.Name}
		for os, p := range r.Platforms {
			if p.LatestVersion == "" {
				continue
			}
			// 没有更新的跳过
			if p.CurrentVersion == p.LatestVersion {
				continue
			}
			item.Platforms = append(item.Platforms, autoPlatform{
				OS:      os,
				Current: p.CurrentVersion,
				Latest:  p.LatestVersion,
				URL:     p.URL,
			})
		}
		if len(item.Platforms) > 0 {
			items = append(items, item)
		}
	}

	if len(items) == 0 {
		fmt.Println("All up to date.")
		return nil
	}

	return runAutoMode(home, items)
}

func runAutoMode(home string, items []autoItem) error {
	var waitList []string
	scanner := bufio.NewScanner(os.Stdin)

	for i, item := range items {
		// 显示
		fmt.Printf("\n%s %s (%s)\n",
			colorCyan(fmt.Sprintf("[%d/%d]", i+1, len(items))),
			colorBold(item.Name),
			colorGray(item.AppID))
		for _, p := range item.Platforms {
			urlStr := formatAutoURL(p.URL)
			fmt.Printf("  %s: %s %s %s  %s\n",
				colorBlue(p.OS),
				colorRed(p.Current),
				colorYellow("→"),
				colorGreen(p.Latest),
				colorGray(urlStr))
		}

		fmt.Print(colorBold("AUTO >>> "))
		if !scanner.Scan() {
			break
		}
		cmd := strings.TrimSpace(scanner.Text())

		switch {
		case cmd == "stop":
			for j := i; j < len(items); j++ {
				waitList = append(waitList, formatWaitItem(items[j]))
			}
			goto done
		case cmd == "skip":
			waitList = append(waitList, formatWaitItem(item))
			for j := i + 1; j < len(items); j++ {
				waitList = append(waitList, formatWaitItem(items[j]))
			}
			goto done
		case cmd == "exit" || cmd == "quit":
			waitList = append(waitList, formatWaitItem(item))
			for j := i + 1; j < len(items); j++ {
				waitList = append(waitList, formatWaitItem(items[j]))
			}
			goto done
		case cmd == "":
			waitList = append(waitList, formatWaitItem(item))
		case cmd == "ok":
			confirmAutoItem(home, item, nil)
		case strings.HasPrefix(cmd, "ok "):
			platforms := strings.Fields(strings.TrimPrefix(cmd, "ok "))
			var okSet []string
			for _, pl := range platforms {
				okSet = append(okSet, pl)
			}
			confirmAutoItem(home, item, okSet)
		default:
			fmt.Println(colorGray("? (ok, ok <platforms>, stop, skip, exit, or Enter to wait)"))
			waitList = append(waitList, formatWaitItem(item))
		}
	}

done:
	if len(waitList) > 0 {
		fmt.Println("\n" + colorYellow("── Wait list ──"))
		for _, w := range waitList {
			fmt.Println(w)
		}
	}
	return nil
}

func confirmAutoItem(home string, item autoItem, okPlatforms []string) {
	body := map[string]string{"app_id": item.AppID}
	for _, p := range item.Platforms {
		if len(okPlatforms) == 0 || contains(okPlatforms, p.OS) {
			body[p.OS] = p.Latest
		}
	}
	b, _ := json.Marshal(body)
	addr := apiAddr(home)
	resp, err := http.Post(addr+"/api/check/confirm", "application/json", strings.NewReader(string(b)))
	if err != nil {
		fmt.Printf("  confirm failed - %v\n", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		raw, _ := io.ReadAll(resp.Body)
		fmt.Printf("  confirm error (%d): %s\n", resp.StatusCode, string(raw))
		return
	}
	for _, p := range item.Platforms {
		if len(okPlatforms) == 0 || contains(okPlatforms, p.OS) {
			fmt.Printf("  %s: %s %s\n", colorBlue(p.OS), colorGreen(p.Latest), colorGreen("✓"))
		}
	}
}

func formatWaitItem(item autoItem) string {
	var parts []string
	for _, p := range item.Platforms {
		parts = append(parts, fmt.Sprintf("%s: %s→%s", p.OS, p.Current, p.Latest))
	}
	return fmt.Sprintf("%s (%s)  %s", item.Name, item.AppID, strings.Join(parts, ", "))
}

func formatAutoURL(u any) string {
	switch v := u.(type) {
	case string:
		if v == "" {
			return ""
		}
		return fmt.Sprintf("[%s]", v)
	case []any:
		if len(v) == 0 {
			return ""
		}
		extra := ""
		if len(v) > 1 {
			extra = fmt.Sprintf(" (+%d)", len(v)-1)
		}
		return fmt.Sprintf("[%v%s]", v[0], extra)
	default:
		return ""
	}
}

func contains(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}
