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

	trackerList, err := store.LoadTracker(home)
	if err != nil {
		return fmt.Errorf("load tracker: %w", err)
	}

	rule, ok := rules[id]
	if !ok {
		return fmt.Errorf("rule %s not found", id)
	}

	var entry *store.TrackerEntry
	for i := range trackerList {
		if trackerList[i].RuleID == id {
			entry = &trackerList[i]
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

	// Direct version set
	if version != "" {
		for _, os := range platforms {
			userData[id][os] = version
		}
		fmt.Printf("%s → %s (all platforms)\n", rule.Info.Name, version)
		return store.SaveUserData(home, userData)
	}

	// Check then ask
	fmt.Printf("%s (%s)\n", rule.Info.Name, id)
	for _, os := range platforms {
		platCfg := rule.MergedConfig(os)
		if platCfg.URL == "" && platCfg.Type != "github" {
			fmt.Printf("  %s: no URL configured\n", os)
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
			fmt.Printf("  %s: check failed - %v\n", os, err)
			continue
		}
		p := resp.Platforms[os]
		latest := p.LatestVersion
		if latest == "" {
			fmt.Printf("  %s: unable to get version\n", os)
			continue
		}

		if current == latest {
			fmt.Printf("  %s: %s [up to date]\n", os, current)
			continue
		}

		fmt.Printf("  %s: %s → %s\n", os, current, latest)
		if askYN("  Update? [y/N] ") {
			userData[id][os] = latest
			fmt.Printf("  Updated\n")
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
