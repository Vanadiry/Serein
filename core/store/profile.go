package store

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/vanadiry/serein/core/httpx"
)

type Profile struct {
	Version         int      `json:"version"`
	KnownExtensions []string `json:"known_extensions"`
	VersionPrefixes []string `json:"version_prefixes"`
	VersionSuffixes []string `json:"version_suffixes"`
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

func SyncProfile(ctx context.Context, home, url string) (Profile, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Profile{}, false, fmt.Errorf("获取 profile.json: %w", err)
	}
	resp, err := httpx.DefaultClient().Do(req)
	if err != nil {
		return Profile{}, false, fmt.Errorf("获取 profile.json: %w", err)
	}
	defer resp.Body.Close()

	if err := httpx.CheckStatus(resp); err != nil {
		return Profile{}, false, fmt.Errorf("获取 profile.json: %w", err)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxFetchBytes))
	if err != nil {
		return Profile{}, false, err
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

	if err := ctx.Err(); err != nil {
		return Profile{}, false, err
	}

	path := filepath.Join(home, "user", "profile.json")
	if err := os.WriteFile(path, body, 0644); err != nil {
		return local, false, err
	}
	return remote, true, nil
}
