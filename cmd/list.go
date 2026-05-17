// serein list：列出所有追踪软件的 id、版本号、规则名称
package cmd

import (
	"fmt"

	"github.com/vanadiry/serein/core/store"
)

func RunList(home string) error {
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
		fmt.Println("No software tracked.")
		return nil
	}

	for _, entry := range trackerList {
		name := "rule not found"
		website := ""
		if rule, ok := rules[entry.AppID]; ok {
			name = rule.Info.Name
			website = rule.Info.OfficialWebsite
		}

		platforms := store.PlatformsFor(entry, cfg.Platforms)
		fmt.Printf("%s  %s", entry.AppID, name)
		if website != "" {
			fmt.Printf("  %s", website)
		}

		versions := userData[entry.AppID]
		for _, os := range platforms {
			ver := ""
			if versions != nil {
				ver = versions[os]
			}
			if ver == "" {
				ver = "-"
			}
			fmt.Printf("  %s:%s", os, ver)
		}
		fmt.Println()
	}
	return nil
}
