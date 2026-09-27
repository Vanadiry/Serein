package store

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/vanadiry/serein/core/log"
)

// 目录与配置文件的权限。config.toml 里有 GitHub token，
// 只需保证，仅当前用户可读，Windows 上权限位简直是个笑话，那不是能靠代码解决的
const (
	dirPerm  os.FileMode = 0700
	filePerm os.FileMode = 0600
)

func Init(home string) error {
	dirs := []string{
		home,
		filepath.Join(home, "rules"),
		filepath.Join(home, "tracker"),
		filepath.Join(home, "user"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, dirPerm); err != nil {
			return fmt.Errorf("mkdir %s: %w", d, err)
		}
		// 目录已存在时 MkdirAll 不会改权限，补一次
		if err := os.Chmod(d, dirPerm); err != nil {
			return fmt.Errorf("chmod %s: %w", d, err)
		}
	}

	if err := initConfigFile(home); err != nil {
		return err
	}

	if err := initProfileFile(home); err != nil {
		return err
	}

	if err := log.InitLogger(home); err != nil {
		return err
	}

	return nil
}

func initConfigFile(home string) error {
	path := filepath.Join(home, "config.toml")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.WriteFile(path, []byte(DefaultConfigTOML), filePerm); err != nil {
			return err
		}
		return nil
	}
	// 已存在的补一次权限：旧版本创建的是 0644
	return os.Chmod(path, filePerm)
}

func initProfileFile(home string) error {
	path := filepath.Join(home, "user", "profile.json")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return os.WriteFile(path, []byte(DefaultProfileJSON), 0644)
	}
	return nil
}
