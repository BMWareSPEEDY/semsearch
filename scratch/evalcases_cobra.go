package main

var cobraCases = []BenchCase{
	// CONCEPTUAL
	{Query: "building a root command for a command line application", Category: "conceptual", Expected: "command.go"},
	{Query: "parsing command line arguments into positional values", Category: "conceptual", Expected: "args.go"},
	{Query: "grouping related flags into a named group for display", Category: "conceptual", Expected: "flag_groups.go"},
	{Query: "generating shell autocompletion for bash zsh fish", Category: "conceptual", Expected: "completions.go"},
	{Query: "showing an error when the wrong number of arguments is passed", Category: "conceptual", Expected: "args.go"},
	{Query: "adding subcommands to build a nested command tree", Category: "conceptual", Expected: "command.go"},
	{Query: "executing the selected command and running its handlers", Category: "conceptual", Expected: "command.go"},
	{Query: "defining flags inherited by every subcommand", Category: "conceptual", Expected: "command.go"},
	{Query: "custom handling when a flag is passed incorrectly", Category: "conceptual", Expected: "command.go"},
	{Query: "suggesting similar commands when the user types a typo", Category: "conceptual", Expected: "command.go"},

	// EXACT SYMBOL
	{Query: "type Command struct", Category: "exact_symbol", Expected: "command.go"},
	{Query: "func (c *Command) Execute", Category: "exact_symbol", Expected: "command.go"},
	{Query: "func (c *Command) AddCommand", Category: "exact_symbol", Expected: "command.go"},
	{Query: "func (c *Command) PersistentFlags", Category: "exact_symbol", Expected: "command.go"},
	{Query: "func (c *Command) Flags", Category: "exact_symbol", Expected: "command.go"},
	{Query: "func (c *Command) SetHelpFunc", Category: "exact_symbol", Expected: "command.go"},
	{Query: "func (c *Command) InitDefaultCompletionCmd", Category: "exact_symbol", Expected: "completions.go"},
	{Query: "func (c *Command) Run", Category: "exact_symbol", Expected: "command.go"},
	{Query: "type Args", Category: "exact_symbol", Expected: "args.go"},
	{Query: "func (c *Command) Find", Category: "exact_symbol", Expected: "command.go"},

	// ERROR HANDLING
	{Query: "propagating command execution errors to the caller", Category: "error_handling", Expected: "command.go"},
	{Query: "returning a friendly error for an unknown subcommand", Category: "error_handling", Expected: "command.go"},

	// RENDERING
	{Query: "rendering the usage text from a custom template", Category: "rendering", Expected: "command.go"},
	{Query: "formatting the help text shown for a command", Category: "rendering", Expected: "command.go"},

	// CONFIG / SETUP
	{Query: "setting a custom help command for the CLI", Category: "config", Expected: "command.go"},
	{Query: "embedding version information reported by the command", Category: "config", Expected: "command.go"},
	{Query: "redirecting command output to a custom writer", Category: "config", Expected: "command.go"},
}
