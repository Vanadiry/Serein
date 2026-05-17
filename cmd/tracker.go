// serein tracker [id]
// 无参数：检查全部追踪
// 有参数：若 tracker 文件不存在则创建，存在则检查该文件内的条目
package cmd

import (
	"fmt"
	"regexp"

	"github.com/vanadiry/serein/core/store"
)

var trackerIDRe = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

func RunTracker(home, name string) error {
	if !trackerIDRe.MatchString(name) {
		return fmt.Errorf("invalid tracker name: %s (only a-z, A-Z, 0-9, -, _ allowed)", name)
	}

	if store.TrackerExists(home, name) {
		return RunTrackerCheck(home, name)
	}

	fmt.Printf("Tracker %s does not exist. Create?\n", name)
	if !askYN("Confirm? [y/N] ") {
		fmt.Println("Cancelled")
		return nil
	}

	if err := store.CreateTrackerFile(home, name); err != nil {
		return fmt.Errorf("create tracker: %w", err)
	}
	fmt.Printf("Created tracker/%s.toml\n", name)
	return nil
}

// RunTrackerCheck 检查指定 tracker 文件内的所有条目
func RunTrackerCheck(home, name string) error {
	entries, err := store.LoadTrackerFile(home, name)
	if err != nil {
		return fmt.Errorf("load tracker: %w", err)
	}
	if len(entries) == 0 {
		fmt.Printf("Tracker %s is empty.\n", name)
		return nil
	}

	cfg, err := store.LoadConfig(home)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	rules, err := store.LoadRules(home)
	if err != nil {
		return fmt.Errorf("load rules: %w", err)
	}

	userData, err := store.LoadUserData(home)
	if err != nil {
		return fmt.Errorf("load user data: %w", err)
	}

	return runChecks(home, entries, cfg, rules, userData)
}
