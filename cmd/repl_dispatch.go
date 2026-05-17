// REPL 命令分发与 AUTO 模式
package cmd

import (
	"fmt"
	"strings"
)

func dispatch(home, line string) error {
	parts := strings.Fields(line)
	if len(parts) == 0 {
		return nil
	}

	switch parts[0] {
	case "help":
		printHelp()
		return nil
	case "tracker":
		return handleTrackerCmd(home, parts[1:])
	case "check":
		return handleCheckCmd(home, parts[1:])
	case "rule":
		return handleRuleCmd(home, parts[1:])
	default:
		return fmt.Errorf("unknown command: %s (type help)", parts[0])
	}
}

func printHelp() {
	fmt.Println(strings.TrimSpace(`
Commands:
  check all [-auto]              Check all tracked software
  check app <ids...> [-auto]     Check specific apps
  check tracker <id> [-auto]     Check tracker entries
  check confirm <id> <version>   Set version for app
  check confirm <id> <os:ver>    Set version per platform
  check all|app|tracker -temp    Show last check result
  rule sync                       Sync rule sources
  rule list                       List all rule sources
  rule list <source_id>           List rules in source
  rule list -s <query>            Search rules
  tracker add <id> [platforms...] Add to _serein tracker
  tracker new <id>                Create new tracker
  tracker list <id>               List tracker details
  tracker list                    List all tracked software
  help                            Show this message
  exit                            Quit REPL
`))
}

