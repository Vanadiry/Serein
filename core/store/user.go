package store

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// UserData 用户追踪数据：app_id → 平台 → 版本号
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

	info, _ := f.Stat()
	if info.Size() == 0 {
		return ud, nil // 空文件 → 返回空数据
	}
	if err := json.NewDecoder(f).Decode(&ud); err != nil && err != io.EOF {
		return nil, fmt.Errorf("decode user data: %w", err)
	}
	return ud, nil
}

// SaveUserData 原子写入：先写临时文件再 rename，避免写一半导致文件损坏
func SaveUserData(home string, ud UserData) error {
	path := filepath.Join(home, "user", "software.json")
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(ud, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "software-*.json.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // 成功后已 rename，删除为 no-op
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
