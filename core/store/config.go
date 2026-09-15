package store

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

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
	Proxy       ProxyConfig    `toml:"proxy"`
	Profile     ProfileConfig  `toml:"profile"`
	RuleSources []RuleSource   `toml:"rule_sources"`
}

// ProxyConfig 上游拉取使用的 HTTP 代理，host 为空则不启用
type ProxyConfig struct {
	Host string `toml:"host"`
	Port int    `toml:"port"`
}

// ProxyURL 返回配置的代理地址；未配置或非法则返回 nil
func (c Config) ProxyURL() *url.URL {
	if c.Proxy.Host == "" || c.Proxy.Port < 1 || c.Proxy.Port > 65535 {
		return nil
	}
	return &url.URL{
		Scheme: "http",
		Host:   net.JoinHostPort(c.Proxy.Host, strconv.Itoa(c.Proxy.Port)),
	}
}

type ProfileConfig struct {
	URL string `toml:"url"`
}

type SereinConfig struct {
	Host     string `toml:"host"`
	Port     int    `toml:"port"`
	FirstRun bool   `toml:"first_run"`
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

func LoadConfig(home string) (Config, error) {
	path := filepath.Join(home, "config.toml")
	var cfg Config
	if err := decodeTOML(path, &cfg); err != nil {
		return cfg, fmt.Errorf("read config: %w", err)
	}
	return cfg, nil
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

// Validate 校验会影响程序正常运行的字段，返回全部错误
func (c Config) Validate() []string {
	var errs []string
	if c.Serein.Host == "" {
		errs = append(errs, "serein.host 不能为空")
	}
	if c.Serein.Port < 1 || c.Serein.Port > 65535 {
		errs = append(errs, fmt.Sprintf("serein.port 必须在 1-65535 之间（当前 %d）", c.Serein.Port))
	}
	if c.Download.Concurrency < 1 {
		errs = append(errs, fmt.Sprintf("download.concurrency 必须 >= 1（当前 %d）", c.Download.Concurrency))
	}
	return errs
}

// ValidationError 汇总配置校验错误
type ValidationError struct {
	Errors []string
}

func (e *ValidationError) Error() string {
	return "配置有误: " + strings.Join(e.Errors, "; ")
}
