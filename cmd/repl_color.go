// REPL 终端颜色
package cmd


type color string

const (
	reset   color = "\033[0m"
	red     color = "\033[31m"
	green   color = "\033[32m"
	yellow  color = "\033[33m"
	blue    color = "\033[34m"
	cyan    color = "\033[36m"
	gray    color = "\033[90m"
	bold    color = "\033[1m"
)

func c(clr color, s string) string {
	return string(clr) + s + string(reset)
}

func colorBold(s string) string  { return c(bold, s) }
func colorRed(s string) string   { return c(red, s) }
func colorGreen(s string) string { return c(green, s) }
func colorYellow(s string) string { return c(yellow, s) }
func colorBlue(s string) string  { return c(blue, s) }
func colorCyan(s string) string  { return c(cyan, s) }
func colorGray(s string) string  { return c(gray, s) }

