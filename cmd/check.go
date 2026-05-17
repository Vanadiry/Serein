// serein check [id]：检查更新
package cmd

import (
	"fmt"
	"strings"

	"github.com/vanadiry/serein/core/checker"
	"github.com/vanadiry/serein/core/store"
)

func RunCheck(home, id string) error {
	cfg, err := store.LoadConfig(home)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	rules, err := store.LoadRules(home)
	if err != nil {
		return fmt.Errorf("load rules: %w", err)
	}
	trackerList, err := store.LoadTracker(home)
	if err != nil {
		return fmt.Errorf("load tracker: %w", err)
	}
	userData, err := store.LoadUserData(home)
	if err != nil {
		return fmt.Errorf("load user data: %w", err)
	}

	if len(trackerList) == 0 {
		fmt.Println("No software tracked. Use serein tracker <id> to add.")
		return nil
	}

	if id != "" {
		var filtered []store.TrackerEntry
		for _, e := range trackerList {
			if e.AppID == id {
				filtered = append(filtered, e)
			}
		}
		if len(filtered) > 0 {
			trackerList = filtered
		}
		// 未找到该 app_id → 检查全部（trackerList 不变）
	}

	return runChecks(home, trackerList, cfg, rules, userData)
}

// runChecks 共享的检查逻辑，供 check、tracker 等命令复用。
func runChecks(home string, entries []store.TrackerEntry, cfg store.Config, rules map[string]store.Rule, userData store.UserData) error {
	checker.ClearURLCache()

	for _, entry := range entries {
		rule, ok := rules[entry.AppID]
		if !ok {
			fmt.Printf("[%s] rule not found\n", entry.AppID)
			continue
		}

		platforms := store.PlatformsFor(entry, cfg.Platforms)
		fmt.Printf("\n%s (%s)\n", rule.Info.Name, entry.AppID)

		for _, os := range platforms {
			platCfg := rule.MergedConfig(os)
			if platCfg.URL == "" && platCfg.Type != "github" {
				fmt.Printf("  %s: no URL configured\n", os)
				continue
			}

			currentVer := ""
			if ud, ok := userData[entry.AppID]; ok {
				currentVer = ud[os]
			}

			req := checker.CheckRequest{
				AppID:     rule.Info.AppID,
				Name:     rule.Info.Name,
				OfficialWebsite: rule.Info.OfficialWebsite,
				RuleType:        platCfg.Type,
				Owner:           platCfg.Owner,
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
				fmt.Printf("  %s: check failed - %v\n", os, err)
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
		fmt.Printf("  %s: unable to get version\n", os)
		if urlStr != "" {
			fmt.Printf("    download: %s\n", urlStr)
		}
		return
	}

	if current == "" {
		fmt.Printf("  %s: %s\n", os, ver)
	} else if current != ver {
		fmt.Printf("  %s: %s → %s [update]\n", os, current, ver)
	} else {
		fmt.Printf("  %s: %s [up to date]\n", os, current)
	}

	if urlStr != "" {
		fmt.Printf("    download: %s\n", urlStr)
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
