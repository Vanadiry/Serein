// serein check：加载配置 → 规则 → tracker → 逐个检查 → 打印结果 → 退出
package cmd

import (
	"fmt"
	"strings"

	"github.com/vanadiry/serein/core/checker"
	"github.com/vanadiry/serein/core/store"
)

func RunCheck(home string) error {
	// 加载配置
	cfg, err := store.LoadConfig(home)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// 加载规则
	rules, err := store.LoadRules(home)
	if err != nil {
		return fmt.Errorf("load rules: %w", err)
	}

	// 加载 tracker
	tracker, err := store.LoadTracker(home)
	if err != nil {
		return fmt.Errorf("load tracker: %w", err)
	}

	// 加载用户数据
	userData, err := store.LoadUserData(home)
	if err != nil {
		return fmt.Errorf("load user data: %w", err)
	}

	if len(tracker.Entries) == 0 {
		fmt.Println("没有追踪的软件。请在 tracker.toml 中添加。")
		return nil
	}

	checker.ClearURLCache()

	for _, entry := range tracker.Entries {
		rule, ok := rules[entry.RuleID]
		if !ok {
			fmt.Printf("[%s] 规则未找到\n", entry.RuleID)
			continue
		}

		platforms := store.PlatformsFor(entry, cfg.Platforms)
		fmt.Printf("\n%s (%s)\n", rule.Info.Name, entry.RuleID)

		for _, os := range platforms {
			platCfg := rule.MergedConfig(os)
			if platCfg.URL == "" && platCfg.Type != "github" {
				fmt.Printf("  %s: 未配置 URL\n", os)
				continue
			}

			currentVer := ""
			if ud, ok := userData[entry.RuleID]; ok {
				currentVer = ud[os]
			}

			req := checker.CheckRequest{
				UUID:     rule.Info.UUID,
				Name:     rule.Info.Name,
				RuleType: platCfg.Type,
				Owner:    platCfg.Owner,
				Repo:     platCfg.Repo,
				Platforms: []checker.PlatformCheckConfig{{
					OS:             os,
					Type:           platCfg.Type,
					URL:            platCfg.URL,
					UA:             platCfg.UA,
					Headers:        platCfg.Headers,
					BaseURL:        platCfg.BaseURL,
					VPosition:      platCfg.VPosition,
					DPosition:      platCfg.DPosition,
					VJoin:          platCfg.VJoin,
					DJoin:          platCfg.DJoin,
					CurrentVersion: currentVer,
				}},
			}

			resp, err := checker.RunCheck(req)
			if err != nil {
				fmt.Printf("  %s: 检查失败 - %v\n", os, err)
				continue
			}
			if p, ok := resp.Platforms[os]; ok {
				printResult(os, currentVer, p)
			}
		}
	}
	return nil
}

func printResult(os, current string, p checker.CheckPlatform) {
	ver := p.LatestVersion
	urlStr := formatURL(p.URL)

	if ver == "" {
		fmt.Printf("  %s: 无法获取版本号\n", os)
		if urlStr != "" {
			fmt.Printf("    下载: %s\n", urlStr)
		}
		return
	}

	if current == "" {
		fmt.Printf("  %s: 最新 %s\n", os, ver)
	} else if current != ver {
		fmt.Printf("  %s: %s → %s [更新]\n", os, current, ver)
	} else {
		fmt.Printf("  %s: %s [已是最新]\n", os, current)
	}

	if urlStr != "" {
		fmt.Printf("    下载: %s\n", urlStr)
	}
}

func formatURL(u any) string {
	switch v := u.(type) {
	case string:
		return v
	case []string:
		return strings.Join(v, "\n          ")
	default:
		return ""
	}
}
