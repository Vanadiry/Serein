package store

import (
	"fmt"
	"os"
	"path/filepath"
)

type TrackerEntry struct {
	RuleID    string   `toml:"rule_id"`
	Platforms []string `toml:"platforms,omitempty"`
}

type Tracker struct {
	Entries []TrackerEntry `toml:"tracker"`
}

func LoadTracker(home string) (Tracker, error) {
	path := filepath.Join(home, "tracker.toml")
	var t Tracker

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return t, nil
	}

	if err := decodeTOML(path, &t); err != nil {
		return t, fmt.Errorf("read tracker: %w", err)
	}
	return t, nil
}

func SaveTracker(home string, t Tracker) error {
	path := filepath.Join(home, "tracker.toml")
	if err := encodeTOML(path, t); err != nil {
		return fmt.Errorf("write tracker: %w", err)
	}
	return nil
}

// PlatformsFor 返回某条追踪实际生效的平台列表。
// tracker 条目有 platforms 则以此为准，否则用 config.platforms。
func PlatformsFor(entry TrackerEntry, cfgPlatforms []string) []string {
	if len(entry.Platforms) > 0 {
		return entry.Platforms
	}
	return cfgPlatforms
}
