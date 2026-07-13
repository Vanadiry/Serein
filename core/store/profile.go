package store

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

type Profile struct {
	Version         int      `json:"version"`
	KnownExtensions []string `json:"known_extensions"`
}

func LoadProfile(home string) (Profile, error) {
	path := filepath.Join(home, "user", "profile.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return Profile{}, err
	}

	var p Profile
	if err := json.Unmarshal(data, &p); err != nil {
		return Profile{}, nil
	}
	return p, nil
}

func SyncProfile(home, url string) (Profile, bool, error) {
	resp, err := http.Get(url)
	if err != nil {
		return Profile{}, false, fmt.Errorf("获取 profile.json: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return Profile{}, false, err
	}

	if resp.StatusCode >= 400 {
		return Profile{}, false, fmt.Errorf("获取 profile.json: HTTP %d", resp.StatusCode)
	}

	var remote Profile
	if err := json.Unmarshal(body, &remote); err != nil {
		return Profile{}, false, fmt.Errorf("解析 profile.json: %w", err)
	}

	local, err := LoadProfile(home)
	if err != nil {
		return Profile{}, false, err
	}

	if remote.Version <= local.Version {
		return local, false, nil
	}

	path := filepath.Join(home, "user", "profile.json")
	if err := os.WriteFile(path, body, 0644); err != nil {
		return local, false, err
	}
	return remote, true, nil
}
