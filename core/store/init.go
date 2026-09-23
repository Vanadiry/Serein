package store

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/vanadiry/serein/core/log"
)

func Init(home string) error {
	dirs := []string{
		home,
		filepath.Join(home, "rules"),
		filepath.Join(home, "tracker"),
		filepath.Join(home, "user"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return fmt.Errorf("mkdir %s: %w", d, err)
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
		return os.WriteFile(path, []byte(DefaultConfigTOML), 0644)
	}
	return nil
}

func initProfileFile(home string) error {
	path := filepath.Join(home, "user", "profile.json")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return os.WriteFile(path, []byte(DefaultProfileJSON), 0644)
	}
	return nil
}
