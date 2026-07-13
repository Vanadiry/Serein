package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type TempCheckPlatform struct {
	CurrentVersion string `json:"current_version,omitempty"`
	LatestVersion  string `json:"latest_version,omitempty"`
	URL            any    `json:"url,omitempty"`
	Error          string `json:"error,omitempty"`
}

type TempCheckResult struct {
	AppID     string                       `json:"app_id"`
	Name      string                       `json:"name"`
	Platforms map[string]TempCheckPlatform `json:"platforms"`
}

type TempCache struct {
	SavedAt int64              `json:"saved_at"`
	Results []TempCheckResult  `json:"results"`
}

func SaveTrackerTemp(home, trackerID string, results []TempCheckResult) error {
	dir := filepath.Join(home, "temp")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	cache := TempCache{
		SavedAt: time.Now().Unix(),
		Results: results,
	}
	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, trackerID+".json"), data, 0644)
}

func LoadTrackerTemp(home, trackerID string) (*TempCache, error) {
	path := filepath.Join(home, "temp", trackerID+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cache TempCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, fmt.Errorf("parse temp cache: %w", err)
	}
	return &cache, nil
}

func IsExpired(cache *TempCache) bool {
	if cache == nil {
		return true
	}
	return time.Now().Unix()-cache.SavedAt > 24*3600
}
