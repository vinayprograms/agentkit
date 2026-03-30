package shellguard

// Posix returns a Shell for POSIX sh.
// POSIX sh is a subset of bash — same metacharacters but no $() or ${}.
func Posix() Shell { return posixShell{} }

type posixShell struct{}

func (posixShell) HasChainedCommands(command string) bool {
	return containsUnquotedMetachars(command,
		[]string{"|", "&&", "||", ";", "`"},
	)
}

func (posixShell) SplitSegments(command string) []string {
	return splitByOperators(command)
}

func (posixShell) ExtractCommand(segment string) string {
	return extractBase(segment)
}
