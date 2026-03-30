package shellguard

import "strings"

// Fish returns a Shell for the fish shell.
// Fish has different syntax: no $() (uses ()), no &&/|| (uses "; and"/"; or"),
// and single quotes don't escape backslashes.
func Fish() Shell { return fishShell{} }

type fishShell struct{}

func (fishShell) HasChainedCommands(command string) bool {
	return containsUnquotedMetachars(command,
		[]string{"|", ";", "(", "`"},
	)
}

func (fishShell) SplitSegments(command string) []string {
	return splitByOperators(command)
}

func (fishShell) ExtractCommand(segment string) string {
	seg := strings.TrimSpace(segment)
	for _, prefix := range []string{"and ", "or "} {
		if strings.HasPrefix(seg, prefix) {
			seg = strings.TrimSpace(seg[len(prefix):])
		}
	}
	return extractBase(seg)
}
