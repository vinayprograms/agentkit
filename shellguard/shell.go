package shellguard

// Shell defines shell-specific command parsing behavior.
// Different shells have different metacharacters, quoting rules, and chaining syntax.
type Shell interface {
	// HasChainedCommands returns true if the command contains shell metacharacters
	// that chain multiple commands (pipes, &&, ||, ;, etc.).
	HasChainedCommands(command string) bool

	// SplitSegments splits a compound command into individual segments
	// by pipes, semicolons, and logical operators.
	SplitSegments(command string) []string

	// ExtractCommand returns the base command name from a single segment,
	// stripping path prefixes, env prefixes, etc.
	ExtractCommand(segment string) string
}
