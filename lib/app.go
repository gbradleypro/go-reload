package gin

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"
)

var (
	errInvalidActionType = errors.New("ERROR invalid Action type. ")
)

// App is the main structure of a cli application. It is recommended that
// an app be created with the cli.NewApp() function
type App struct {
	// The name of the program. Defaults to path.Base(os.Args[0])
	Name string
	// Full name of command for help, defaults to Name
	HelpName string
	// Description of the program.
	Usage string
	// Text to override the USAGE section of help
	UsageText string
	// Description of the program argument format.
	ArgsUsage string
	// Version of the program
	Version string
	// Description of the program
	Description string
	// List of commands to execute
	Commands []Command
	// List of flags to parse
	Flags []Flag
	// Boolean to enable bash completion commands
	EnableBashCompletion bool
	// Boolean to hide built-in help command
	HideHelp bool
	// Boolean to hide built-in version flag and the VERSION section of help
	HideVersion bool
	// Populate on app startup, only gettable through method Categories()
	categories CommandCategories
	// An action to execute when the bash-completion flag is set
	BashComplete BashCompleteFunc
	// An action to execute before any subcommands are run, but after the context is ready
	// If a non-nil error is returned, no subcommands are run
	Before BeforeFunc
	// An action to execute after any subcommands are run, but after the subcommand has finished
	// It is run even if Action() panics
	After AfterFunc

	// The action to execute when no subcommands are specified
	// Expects a `cli.ActionFunc` but will accept the *deprecated* signature of `func(*cli.Context) {}`
	// *Note*: support for the deprecated `Action` signature will be removed in a future version
	Action interface{}

	// Execute this function if the proper command cannot be found
	CommandNotFound CommandNotFoundFunc
	// Execute this function if a usage error occurs
	OnUsageError OnUsageErrorFunc
	// Compilation date
	Compiled time.Time
	// List of all authors who contributed
	Authors []author
	// Copyright of the binary if any
	Copyright string
	// Name of Author (Note: Use App.Authors, this is deprecated)
	Author string
	// Email of Author (Note: Use App.Authors, this is deprecated)
	Email string
	// Writer to write output to
	Writer io.Writer
	// ErrWriter writes error output
	ErrWriter io.Writer
	// Execute this function to handle ExitErrors. If not provided, HandleExitCoder is provided to
	// function as a default, so this is optional.
	ExitErrHandler ExitErrHandlerFunc
	// Other custom info
	Metadata map[string]interface{}
	// Carries a function which returns app specific info.
	ExtraInfo func() map[string]string
	// CustomAppHelpTemplate the text template for app help topic.
	// cli.go uses text/template to render templates. You can
	// render custom help text by setting this variable.
	CustomAppHelpTemplate string
	// Boolean to enable short-option handling so user can combine several
	// single-character bool arguments into one
	// i.e. foobar -o -v -> foobar -ov
	UseShortOptionHandling bool

	didSetup bool
}

// Tries to find out when this binary was compiled.
// Returns the current time if it fails to find it.
func compileTime() time.Time {
	info, err := os.Stat(os.Args[0])
	if err != nil {
		return time.Now()
	}
	return info.ModTime()
}

// NewApp creates a new cli Application with some reasonable defaults for Name,
// Usage, Version and Action.
func NewApp() *App {
	return &App{
		Name:         filepath.Base(os.Args[0]),
		HelpName:     filepath.Base(os.Args[0]),
		Usage:        "A new cli application",
		UsageText:    "",
		BashComplete: defaultAppComplete,
		Action:       helpCommand.Action,
		Compiled:     compileTime(),
		Writer:       os.Stdout,
	}
}

// setup runs initialization code to ensure all data structures are ready for
// `Run` or inspection prior to `Run`.  It is internally called by `Run`, but
// will return early if setup has already happened.
func (a *App) setup() {
	if a.didSetup {
		return
	}

	a.didSetup = true

	if a.Author != "" || a.Email != "" {
		a.Authors = append(a.Authors, author{Name: a.Author, Email: a.Email})
	}

	var newCMDs []Command
	for _, c := range a.Commands {
		if c.HelpName == "" {
			c.HelpName = fmt.Sprintf("%s %s", a.HelpName, c.Name)
		}
		newCMDs = append(newCMDs, c)
	}
	a.Commands = newCMDs

	if a.command(helpCommand.Name) == nil && !a.HideHelp {
		a.Commands = append(a.Commands, helpCommand)
		if (HelpFlag != BoolFlag{}) {
			a.appendFlag(HelpFlag)
		}
	}

	if a.Version == "" {
		a.HideVersion = true
	}

	if !a.HideVersion {
		a.appendFlag(VersionFlag)
	}

	a.categories = CommandCategories{}
	for _, command := range a.Commands {
		a.categories = a.categories.AddCommand(command.Category, command)
	}
	sort.Sort(a.categories)

	if a.Metadata == nil {
		a.Metadata = make(map[string]interface{})
	}

	if a.Writer == nil {
		a.Writer = os.Stdout
	}
}

func (a *App) newFlagSet() (*flag.FlagSet, error) {
	return flagSet(a.Name, a.Flags)
}

func (a *App) useShortOptionHandling() bool {
	return a.UseShortOptionHandling
}

// Run is the entry point to the cli app. Parses the arguments slice and routes
// to the proper flag/args combination
func (a *App) Run(arguments []string) (err error) {
	a.setup()

	// handle the completion flag separately from the FlagSet since
	// completion could be attempted after a flag, but before its value was put
	// on the command line. this causes the FlagSet to interpret the completion
	// flag name as the value of the flag before it which is undesirable
	// note that we can only do this because the shell autocomplete function
	// always appends the completion flag at the end of the command
	shellComplete, arguments := checkShellCompleteFlag(a, arguments)

	set, err := a.newFlagSet()
	if err != nil {
		return err
	}

	err = parseIter(set, a, arguments[1:], shellComplete)
	nerr := normalizeFlags(a.Flags, set)
	context := NewContext(a, set, nil)
	if nerr != nil {
		_, _ = fmt.Fprintln(a.Writer, nerr)
		return nerr
	}
	context.shellComplete = shellComplete

	if checkCompletions(context) {
		return nil
	}

	if err != nil {
		if a.OnUsageError != nil {
			err := a.OnUsageError(context, err, false)
			a.handleExitCoder(context, err)
			return err
		}
		_, _ = fmt.Fprintf(a.Writer, "%s %s\n\n", "Incorrect Usage.", err.Error())
		return err
	}

	cerr := checkRequiredFlags(a.Flags, context)
	if cerr != nil {
		return cerr
	}

	if a.After != nil {
		defer func() {
			if afterErr := a.After(context); afterErr != nil {
				if err == nil {
					err = afterErr
				}
			}
		}()
	}

	if a.Before != nil {
		beforeErr := a.Before(context)
		if beforeErr != nil {
			a.handleExitCoder(context, beforeErr)
			err = beforeErr
			return err
		}
	}

	args := context.Args()
	if args.Present() {
		name := args.First()
		c := a.command(name)
		if c != nil {
			return c.run(context)
		}
	}

	if a.Action == nil {
		a.Action = helpCommand.Action
	}

	// Run default Action
	err = handleAction(a.Action, context)

	a.handleExitCoder(context, err)
	return err
}

// runAsSubcommand invokes the subcommand given the context, parses ctx.Args() to
// generate command-specific flags
func (a *App) runAsSubcommand(ctx *Context) (err error) {
	// append help to command
	if len(a.Commands) > 0 {
		if a.command(helpCommand.Name) == nil && !a.HideHelp {
			a.Commands = append(a.Commands, helpCommand)
			if (HelpFlag != BoolFlag{}) {
				a.appendFlag(HelpFlag)
			}
		}
	}

	var newCMDs []Command
	for _, c := range a.Commands {
		if c.HelpName == "" {
			c.HelpName = fmt.Sprintf("%s %s", a.HelpName, c.Name)
		}
		newCMDs = append(newCMDs, c)
	}
	a.Commands = newCMDs

	set, err := a.newFlagSet()
	if err != nil {
		return err
	}

	err = parseIter(set, a, ctx.Args().Tail(), ctx.shellComplete)
	nerr := normalizeFlags(a.Flags, set)
	context := NewContext(a, set, ctx)

	if nerr != nil {
		_, _ = fmt.Fprintln(a.Writer, nerr)
		_, _ = fmt.Fprintln(a.Writer)

		return nerr
	}

	if checkCompletions(context) {
		return nil
	}

	if err != nil {
		if a.OnUsageError != nil {
			err = a.OnUsageError(context, err, true)
			a.handleExitCoder(context, err)
			return err
		}
		_, _ = fmt.Fprintf(a.Writer, "%s %s\n\n", "Incorrect Usage.", err.Error())

		return err
	}

	if len(a.Commands) > 0 {
		if checkSubcommandHelp(context) {
			return nil
		}
	}

	cerr := checkRequiredFlags(a.Flags, context)
	if cerr != nil {
		return cerr
	}

	if a.After != nil {
		defer func() {
			afterErr := a.After(context)
			if afterErr != nil {
				a.handleExitCoder(context, err)
				if err == nil {
					err = afterErr
				}
			}
		}()
	}

	if a.Before != nil {
		beforeErr := a.Before(context)
		if beforeErr != nil {
			a.handleExitCoder(context, beforeErr)
			err = beforeErr
			return err
		}
	}

	args := context.Args()
	if args.Present() {
		name := args.First()
		c := a.command(name)
		if c != nil {
			return c.run(context)
		}
	}

	// Run default Action
	err = handleAction(a.Action, context)

	a.handleExitCoder(context, err)
	return err
}

// Command returns the named command on App. Returns nil if the command does not exist
func (a *App) command(name string) *Command {
	for _, c := range a.Commands {
		if c.hasName(name) {
			return &c
		}
	}

	return nil
}

// Categories returns a slice containing all the categories with the commands they contain
func (a *App) Categories() CommandCategories {
	return a.categories
}

// visibleCategories returns a slice of categories and commands that are
// Hidden=false
func (a *App) visibleCategories() []*CommandCategory {
	var ret []*CommandCategory
	for _, category := range a.categories {
		if visible := func() *CommandCategory {
			for _, command := range category.Commands {
				if !command.Hidden {
					return category
				}
			}
			return nil
		}(); visible != nil {
			ret = append(ret, visible)
		}
	}
	return ret
}

// VisibleCommands returns a slice of the commands with Hidden=false
func (a *App) VisibleCommands() []Command {
	var ret []Command
	for _, command := range a.Commands {
		if !command.Hidden {
			ret = append(ret, command)
		}
	}
	return ret
}

// VisibleFlags returns a slice of the Flags with Hidden=false
func (a *App) VisibleFlags() []Flag {
	return visibleFlags(a.Flags)
}

func (a *App) hasFlag(flag Flag) bool {
	for _, f := range a.Flags {
		if flag == f {
			return true
		}
	}

	return false
}

func (a *App) appendFlag(flag Flag) {
	if !a.hasFlag(flag) {
		a.Flags = append(a.Flags, flag)
	}
}

func (a *App) handleExitCoder(context *Context, err error) {
	if a.ExitErrHandler != nil {
		a.ExitErrHandler(context, err)
	}
}

// author represents someone who has contributed to a cli project.
type author struct {
	Name  string // The Authors name
	Email string // The Authors email
}

// String makes author comply to the Stringer interface, to allow an easy print in the templating process
func (a author) String() string {
	e := ""
	if a.Email != "" {
		e = " <" + a.Email + ">"
	}

	return fmt.Sprintf("%v%v", a.Name, e)
}

// handleAction attempts to figure out which Action signature was used.  If
// it's an ActionFunc or a func with the legacy signature for Action, the func
// is run!
func handleAction(action interface{}, context *Context) (err error) {
	switch a := action.(type) {
	case ActionFunc:
		return a(context)
	case func(*Context) error:
		return a(context)
	case func(*Context): // deprecated function signature
		a(context)
		return nil
	}

	return errInvalidActionType
}
