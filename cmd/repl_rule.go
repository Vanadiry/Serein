// REPL rule 命令
package cmd

import (
	"fmt"
	"strings"
)

func handleRuleCmd(home string, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: rule <sync|list> ...")
	}

	switch args[0] {
	case "sync":
		return apiPost(home, "/api/rules/sync", []byte("{}"))
	case "list":
		if len(args) > 2 && args[1] == "-s" {
			return apiGet(home, "/api/rules/list/search?q="+strings.Join(args[2:], " "))
		}
		if len(args) > 1 && !strings.HasPrefix(args[1], "-") {
			return apiGet(home, "/api/rules/list/"+args[1])
		}
		return apiGet(home, "/api/rules/list/all")
	default:
		return fmt.Errorf("unknown rule subcommand: %s", args[0])
	}
}
