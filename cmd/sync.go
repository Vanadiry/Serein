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
		fmt.Println("No rule sources configured. Add [[rule_sources]] to config.toml.")
		return nil
	}

	result := store.SyncAllSources(home, cfg.RuleSources)

	for _, id := range result.Synced {
		fmt.Printf("Synced  %s\n", id)
	}
	for _, e := range result.Errors {
		fmt.Printf("Error  %s: %s\n", e.URL, e.Reason)
	}
	return nil
}
