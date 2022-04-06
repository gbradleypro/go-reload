package gin

import (
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"
)

var helpCommand = Command{
	Name:      "help",
	Aliases:   []string{"h"},
	Usage:     "Shows a list of commands or help for one command",
	ArgsUsage: "[command]",
	Action: func(c *Context) error {
		args := c.Args()
		if args.Present() {
			return nil
		}

		return nil
	},
}

var helpSubcommand = Command{
	Name:      "help",
	Aliases:   []string{"h"},
	Usage:     "Shows a list of commands or help for one command",
	ArgsUsage: "[command]",
	Action: func(c *Context) error {
		args := c.Args()
		if args.Present() {
			return nil
		}

		return nil
	},
}

// defaultAppComplete prints the list of subcommands as the default app completion method
func defaultAppComplete(c *Context) {
	defaultCompleteWithFlags(nil)(c)
}

func printCommandSuggestions(commands []Command, writer io.Writer) {
	for _, command := range commands {
		if command.Hidden {
			continue
		}
		if os.Getenv("_CLI_ZSH_AUTOCOMPLETE_HACK") == "1" {
			for _, name := range command.Names() {
				_, _ = fmt.Fprintf(writer, "%s:%s\n", name, command.Usage)
			}
		} else {
			for _, name := range command.Names() {
				_, _ = fmt.Fprintf(writer, "%s\n", name)
			}
		}
	}
}

func cliArgContains(flagName string) bool {
	for _, name := range strings.Split(flagName, ",") {
		name = strings.TrimSpace(name)
		count := utf8.RuneCountInString(name)
		if count > 2 {
			count = 2
		}
		flag := fmt.Sprintf("%s%s", strings.Repeat("-", count), name)
		for _, a := range os.Args {
			if a == flag {
				return true
			}
		}
	}
	return false
}

func printFlagSuggestions(lastArg string, flags []Flag, writer io.Writer) {
	cur := strings.TrimPrefix(lastArg, "-")
	cur = strings.TrimPrefix(cur, "-")
	for _, flag := range flags {
		if bflag, ok := flag.(BoolFlag); ok && bflag.Hidden {
			continue
		}
		for _, name := range strings.Split(flag.GetName(), ",") {
			name = strings.TrimSpace(name)
			// this will get total count utf8 letters in flag name
			count := utf8.RuneCountInString(name)
			if count > 2 {
				count = 2 // reuse this count to generate single - or -- in flag completion
			}
			// if flag name has more than one utf8 letter and last argument in cli has -- prefix then
			// skip flag completion for short flags example -v or -x
			if strings.HasPrefix(lastArg, "--") && count == 1 {
				continue
			}
			// match if last argument matches this flag, and it is not repeated
			if strings.HasPrefix(name, cur) && cur != name && !cliArgContains(flag.GetName()) {
				flagCompletion := fmt.Sprintf("%s%s", strings.Repeat("-", count), name)
				_, _ = fmt.Fprintln(writer, flagCompletion)
			}
		}
	}
}

func defaultCompleteWithFlags(cmd *Command) func(c *Context) {
	return func(c *Context) {
		if len(os.Args) > 2 {
			lastArg := os.Args[len(os.Args)-2]
			if strings.HasPrefix(lastArg, "-") {
				printFlagSuggestions(lastArg, c.App.Flags, c.App.Writer)
				if cmd != nil {
					printFlagSuggestions(lastArg, cmd.Flags, c.App.Writer)
				}
				return
			}
		}
		if cmd != nil {
			printCommandSuggestions(cmd.Subcommands, c.App.Writer)
		} else {
			printCommandSuggestions(c.App.Commands, c.App.Writer)
		}
	}
}

// showCompletions prints the lists of commands within a given context
func showCompletions(c *Context) {
	a := c.App
	if a != nil && a.BashComplete != nil {
		a.BashComplete(c)
	}
}

// showCommandCompletions prints the custom completions for a given command
func showCommandCompletions(ctx *Context, command string) {
	c := ctx.App.command(command)
	if c != nil {
		if c.BashComplete != nil {
			c.BashComplete(ctx)
		} else {
			defaultCompleteWithFlags(c)(ctx)
		}
	}

}

func checkSubcommandHelp(c *Context) bool {
	if c.Bool("h") || c.Bool("help") {
		return true
	}

	return false
}

func checkShellCompleteFlag(a *App, arguments []string) (bool, []string) {
	if !a.EnableBashCompletion {
		return false, arguments
	}

	pos := len(arguments) - 1
	lastArg := arguments[pos]

	if lastArg != "--"+BashCompletionFlag.GetName() {
		return false, arguments
	}

	return true, arguments[:pos]
}

func checkCompletions(c *Context) bool {
	if !c.shellComplete {
		return false
	}

	if args := c.Args(); args.Present() {
		name := args.First()
		if cmd := c.App.command(name); cmd != nil {
			// let the command handle the completion
			return false
		}
	}

	showCompletions(c)
	return true
}

func checkCommandCompletions(c *Context, name string) bool {
	if !c.shellComplete {
		return false
	}

	showCommandCompletions(c, name)
	return true
}
