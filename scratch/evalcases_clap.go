package main

var clapCases = []BenchCase{
	// CONCEPTUAL
	{Query: "declaring a subcommand on a command", Category: "conceptual", Expected: "clap_builder/src/builder/command.rs"},
	{Query: "registering a positional argument on a command", Category: "conceptual", Expected: "clap_builder/src/builder/arg.rs"},
	{Query: "defining a short and long flag for an option", Category: "conceptual", Expected: "clap_builder/src/builder/arg.rs"},
	{Query: "parsing the command line into matches", Category: "conceptual", Expected: "clap_builder/src/builder/command.rs"},
	{Query: "fallible parsing of arguments into matches", Category: "conceptual", Expected: "clap_builder/src/builder/command.rs"},
	{Query: "grouping arguments so at least one must be present", Category: "conceptual", Expected: "clap_builder/src/builder/arg_group.rs"},
	{Query: "validating that required arguments were supplied", Category: "conceptual", Expected: "clap_builder/src/parser/validator.rs"},
	{Query: "checking for conflicting arguments during validation", Category: "conceptual", Expected: "clap_builder/src/parser/validator.rs"},
	{Query: "attaching a help string to an argument", Category: "conceptual", Expected: "clap_builder/src/builder/arg.rs"},
	{Query: "setting a default value for a missing argument", Category: "conceptual", Expected: "clap_builder/src/builder/arg.rs"},
	{Query: "specifying how many values an argument takes", Category: "conceptual", Expected: "clap_builder/src/builder/arg.rs"},
	{Query: "parsing an argument value with a custom parser", Category: "conceptual", Expected: "clap_builder/src/builder/value_parser.rs"},
	{Query: "rendering the help text template for a command", Category: "conceptual", Expected: "clap_builder/src/output/help_template.rs"},
	{Query: "generating the usage line for a command", Category: "conceptual", Expected: "clap_builder/src/output/usage.rs"},
	{Query: "mapping a parsing failure to an error kind", Category: "conceptual", Expected: "clap_builder/src/error/kind.rs"},
	{Query: "short name aliases for an argument", Category: "conceptual", Expected: "clap_builder/src/builder/arg.rs"},
	{Query: "deriving a subcommand enum from a rust enum", Category: "conceptual", Expected: "clap_derive/src/derives/subcommand.rs"},
	{Query: "generating shell completion scripts for a command", Category: "conceptual", Expected: "clap_complete/src/aot/generator/mod.rs"},
	{Query: "lexing short flag clusters like -abc", Category: "conceptual", Expected: "clap_lex/src/lib.rs"},

	// EXACT SYMBOL
	{Query: "pub fn subcommand", Category: "exact_symbol", Expected: "clap_builder/src/builder/command.rs"},
	{Query: "pub fn short", Category: "exact_symbol", Expected: "clap_builder/src/builder/arg.rs"},
	{Query: "pub fn long", Category: "exact_symbol", Expected: "clap_builder/src/builder/arg.rs"},
	{Query: "pub fn required", Category: "exact_symbol", Expected: "clap_builder/src/builder/arg.rs"},
	{Query: "pub fn default_value", Category: "exact_symbol", Expected: "clap_builder/src/builder/arg.rs"},
	{Query: "pub fn num_args", Category: "exact_symbol", Expected: "clap_builder/src/builder/arg.rs"},
	{Query: "pub fn action", Category: "exact_symbol", Expected: "clap_builder/src/builder/arg.rs"},
	{Query: "pub enum ErrorKind", Category: "exact_symbol", Expected: "clap_builder/src/error/kind.rs"},
	{Query: "struct ArgGroup", Category: "exact_symbol", Expected: "clap_builder/src/builder/arg_group.rs"},
	{Query: "pub fn validate_required", Category: "exact_symbol", Expected: "clap_builder/src/parser/validator.rs"},
	{Query: "pub fn group", Category: "exact_symbol", Expected: "clap_builder/src/builder/command.rs"},
	{Query: "struct ValueParser", Category: "exact_symbol", Expected: "clap_builder/src/builder/value_parser.rs"},
	{Query: "pub fn version", Category: "exact_symbol", Expected: "clap_builder/src/builder/command.rs"},
	{Query: "pub fn about", Category: "exact_symbol", Expected: "clap_builder/src/builder/command.rs"},

	// ERROR HANDLING
	{Query: "returning a clap error when parsing fails", Category: "error_handling", Expected: "clap_builder/src/error"},
	{Query: "formatting an error message for display", Category: "error_handling", Expected: "clap_builder/src/error/format.rs"},

	// RENDERING
	{Query: "writing colored help output with styling", Category: "rendering", Expected: "clap_builder/src/builder/styling.rs"},
	{Query: "wrapping help text to fit terminal width", Category: "rendering", Expected: "clap_builder/src/output/textwrap"},
	{Query: "printing help and version on demand", Category: "rendering", Expected: "clap_builder/src/output/help.rs"},

	// CONFIG
	{Query: "enabling an application setting flag", Category: "config", Expected: "clap_builder/src/builder/app_settings.rs"},
	{Query: "requiring a subcommand to be chosen", Category: "config", Expected: "clap_builder/src/builder/command.rs"},
}
