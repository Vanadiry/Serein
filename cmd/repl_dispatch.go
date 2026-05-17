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
	fmt.Println(colorBold("Commands:"))
	fmt.Println(`  ` + colorGreen("check all [-auto]") + `              Check all tracked software`)
	fmt.Println(`  ` + colorGreen("check app <ids...> [-auto]") + `     Check specific apps`)
	fmt.Println(`  ` + colorGreen("check tracker <id> [-auto]") + `     Check tracker entries`)
	fmt.Println(`  ` + colorGreen("check confirm <id> <version>") + `   Set version for app`)
	fmt.Println(`  ` + colorGreen("check confirm <id> <os:ver>") + `    Set version per platform`)
	fmt.Println(`  ` + colorGreen("check all|app|tracker -temp") + `    Show last check result`)
	fmt.Println(`  ` + colorCyan("rule sync") + `                       Sync rule sources`)
	fmt.Println(`  ` + colorCyan("rule list") + `                       List all rule sources`)
	fmt.Println(`  ` + colorCyan("rule list <source_id>") + `           List rules in source`)
	fmt.Println(`  ` + colorCyan("rule list -s <query>") + `            Search rules`)
	fmt.Println(`  ` + colorYellow("tracker add <id> [platforms...]") + ` Add to _serein tracker`)
	fmt.Println(`  ` + colorYellow("tracker new <id>") + `              Create new tracker`)
	fmt.Println(`  ` + colorYellow("tracker list <id>") + `             List tracker details`)
	fmt.Println(`  ` + colorYellow("tracker list") + `                  List all tracked software`)
	fmt.Println(`  ` + colorGray("help") + `                            Show this message`)
	fmt.Println(`  ` + colorGray("exit") + `                            Quit REPL`)
}

