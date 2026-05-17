// REPL 交互式终端
package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

func RunREPL(home string) error {
	fmt.Println("Serein REPL")
	fmt.Println("Type help for commands, exit to quit.")
	scanner := bufio.NewScanner(os.Stdin)

	for {
		fmt.Print(">>> ")
		if !scanner.Scan() {
			break
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if line == "exit" || line == "quit" {
			fmt.Println("Bye.")
			return nil
		}
		if err := dispatch(home, line); err != nil {
			fmt.Printf("Error: %v\n", err)
		}
	}
	return scanner.Err()
}
