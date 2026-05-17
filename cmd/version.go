// serein version <id> [version]
// 无 version 参数：检查最新版本，询问后写入。
// 有 version 参数：直接写入。
package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/vanadiry/serein/core/checker"
	"github.com/vanadiry/serein/core/store"
)

func RunVersion(home, id, version string) error {
	cfg, err := store.LoadConfig(home)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	rules, err := store.LoadRules(home)
	if err != nil {
		return fmt.Errorf("load rules: %w", err)
	}

	tracker, err := store.LoadTracker(home)
	if err != nil {
		return fmt.Errorf("load tracker: %w", err)
	}

	rule, ok := rules[id]
	if !ok {
		return fmt.Errorf("规则 %s 未找到", id)
	}

	// 查找 entry 确定平台
	var entry *store.TrackerEntry
	for i := range tracker.Entries {
		if tracker.Entries[i].RuleID == id {
			entry = &tracker.Entries[i]
			break
		}
	}

	platforms := rule.Info.Platforms
	if entry != nil {
		platforms = store.PlatformsFor(*entry, cfg.Platforms)
	}

	userData, err := store.LoadUserData(home)
	if err != nil {
		return fmt.Errorf("load user data: %w", err)
	}

	if userData[id] == nil {
		userData[id] = make(map[string]string)
	}

	// 直接指定版本号
	if version != "" {
		for _, os := range platforms {
			userData[id][os] = version
		}
		fmt.Printf("%s → %s (所有平台)\n", rule.Info.Name, version)
		return store.SaveUserData(home, userData)
	}

	// 无版本号：检查后询问
	fmt.Printf("%s (%s)\n", rule.Info.Name, id)
	for _, os := range platforms {
		platCfg := rule.MergedConfig(os)
		if platCfg.URL == "" && platCfg.Type != "github" {
			fmt.Printf("  %s: 未配置 URL\n", os)
			continue
		}

		current := userData[id][os]
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
				CurrentVersion: current,
			}},
		}

		resp, err := checker.RunCheck(req)
		if err != nil {
			fmt.Printf("  %s: 检查失败 - %v\n", os, err)
			continue
		}
		p := resp.Platforms[os]
		latest := p.LatestVersion
		if latest == "" {
			fmt.Printf("  %s: 无法获取版本号\n", os)
			continue
		}

		if current == latest {
			fmt.Printf("  %s: %s [已是最新]\n", os, current)
			continue
		}

		fmt.Printf("  %s: %s → %s\n", os, current, latest)
		if askYN("  确认更新? [y/N] ") {
			userData[id][os] = latest
			fmt.Printf("  已更新\n")
		}
	}
	return store.SaveUserData(home, userData)
}

func askYN(prompt string) bool {
	fmt.Print(prompt)
	r := bufio.NewReader(os.Stdin)
	line, _ := r.ReadString('\n')
	line = strings.TrimSpace(strings.ToLower(line))
	return line == "y" || line == "yes"
}
