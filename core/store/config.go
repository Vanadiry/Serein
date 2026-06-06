package store

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

type RuleSource struct {
	URL string `toml:"url"`
}

type Config struct {
	Serein      SereinConfig   `toml:"serein"`
	Tracker     TrackerConfig  `toml:"tracker"`
	Download    DownloadConfig `toml:"download"`
	Access      AccessConfig   `toml:"access"`
	RuleSources []RuleSource   `toml:"rule_sources"`
}

type SereinConfig struct {
	Host string `toml:"host"`
	Port int    `toml:"port"`
}

type TrackerConfig struct {
	Platforms []string `toml:"platforms"`
}

type DownloadConfig struct {
	Concurrency int    `toml:"concurrency"`
	Downloader  string `toml:"downloader,omitempty"`
}

type AccessConfig struct {
	GithubToken string `toml:"github_token,omitempty"`
}

func DefaultConfig() Config {
	return Config{
		Serein: SereinConfig{
			Host: "127.0.0.1",
			Port: 12510,
		},
		Tracker: TrackerConfig{
			Platforms: []string{"macos", "windows"},
		},
		Download: DownloadConfig{
			Concurrency: 8,
		},
	}
}

func LoadConfig(home string) (Config, error) {
	path := filepath.Join(home, "config.toml")
	cfg := DefaultConfig()

	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.WriteFile(path, []byte(DefaultConfigTOML), 0644); err != nil {
			return cfg, fmt.Errorf("create default config: %w", err)
		}
		return cfg, nil
	}

	if err := decodeTOML(path, &cfg); err != nil {
		return cfg, fmt.Errorf("read config: %w", err)
	}
	return cfg, nil
}

func SaveConfig(home string, cfg Config) error {
	path := filepath.Join(home, "config.toml")
	if err := encodeTOML(path, cfg); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

func decodeTOML(path string, v any) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = toml.NewDecoder(f).Decode(v)
	return err
}

func encodeTOML(path string, v any) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(v)
}
