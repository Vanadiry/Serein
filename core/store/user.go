package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// UserData 用户追踪数据：rule_id → 平台 → 版本号
type UserData map[string]map[string]string

func LoadUserData(home string) (UserData, error) {
	path := filepath.Join(home, "user", "software.json")
	ud := make(UserData)

	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ud, nil
		}
		return nil, fmt.Errorf("read user data: %w", err)
	}
	defer f.Close()

	if err := json.NewDecoder(f).Decode(&ud); err != nil {
		return nil, fmt.Errorf("decode user data: %w", err)
	}
	return ud, nil
}

func SaveUserData(home string, ud UserData) error {
	path := filepath.Join(home, "user", "software.json")
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(ud)
}
