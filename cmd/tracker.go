// serein tracker <rule_id>
// 创建 tracker。若已存在则不操作，若不存在则询问后创建。
package cmd

import (
	"fmt"
	"regexp"

	"github.com/vanadiry/serein/core/store"
)

var trackerIDRe = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

func RunTracker(home, ruleID string) error {
	if !trackerIDRe.MatchString(ruleID) {
		return fmt.Errorf("invalid tracker id: %s (only a-z, A-Z, 0-9, -, _ allowed)", ruleID)
	}

	if store.TrackerExists(home, ruleID) {
		name := ruleID
		rules, _ := store.LoadRules(home)
		if rule, ok := rules[ruleID]; ok {
			name = rule.Info.Name
		}
		fmt.Printf("%s is already tracked\n", name)
		return nil
	}

	// 检查对应规则是否存在
	rules, _ := store.LoadRules(home)
	rule, ruleFound := rules[ruleID]

	if ruleFound {
		fmt.Printf("Track: %s (%s)\n", rule.Info.Name, ruleID)
	} else {
		fmt.Printf("Rule %s not found.\n", ruleID)
		fmt.Printf("Create tracker anyway? (no matching rule, check will fail)\n")
	}
	if !askYN("Confirm? [y/N] ") {
		fmt.Println("Cancelled")
		return nil
	}

	if err := store.CreateTracker(home, ruleID, nil); err != nil {
		return fmt.Errorf("create tracker: %w", err)
	}
	fmt.Printf("Added %s\n", ruleID)
	return nil
}
