// serein sync：拉取所有规则源更新
package cmd

import (
	"fmt"

	"github.com/vanadiry/serein/core/store"
)

func RunSync(home string) error {
	cfg, err := store.LoadConfig(home)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if len(cfg.RuleSources) == 0 {
		fmt.Println("没有配置规则源。请在 config.toml 中添加 [[rule_sources]]。")
		return nil
	}

	result := store.SyncAllSources(home, cfg.RuleSources)

	for _, id := range result.Synced {
		fmt.Printf("已同步  %s\n", id)
	}
	for _, e := range result.Errors {
		fmt.Printf("错误  %s: %s\n", e.URL, e.Reason)
	}
	return nil
}
