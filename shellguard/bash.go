package shellguard

import "strings"

// Bash returns a Shell for bash and zsh (identical parsing rules).
func Bash() Shell { return bashShell{} }

type bashShell struct{}

func (bashShell) HasChainedCommands(command string) bool {
	return containsUnquotedMetachars(command,
		[]string{"|", "&&", "||", ";", "`", "$(", "${"},
	)
}

func (bashShell) SplitSegments(command string) []string {
	return splitByOperators(command)
}

func (bashShell) ExtractCommand(segment string) string {
	return extractBase(segment)
}

// containsUnquotedMetachars checks for metacharacters outside of quotes.
func containsUnquotedMetachars(cmd string, metachars []string) bool {
	inSingle := false
	inDouble := false

	for i := 0; i < len(cmd); i++ {
		c := cmd[i]
		if c == '\'' && !inDouble {
			inSingle = !inSingle
		} else if c == '"' && !inSingle {
			inDouble = !inDouble
		}

		if !inSingle && !inDouble {
			remaining := cmd[i:]
			for _, meta := range metachars {
				if strings.HasPrefix(remaining, meta) {
					return true
				}
			}
		}
	}

	return false
}

// splitByOperators splits a command by pipes, semicolons, and logical operators,
// respecting quotes.
func splitByOperators(cmd string) []string {
	var segments []string
	current := ""
	inSingle := false
	inDouble := false

	for i := 0; i < len(cmd); i++ {
		c := cmd[i]
		if c == '\'' && !inDouble {
			inSingle = !inSingle
			current += string(c)
		} else if c == '"' && !inSingle {
			inDouble = !inDouble
			current += string(c)
		} else if !inSingle && !inDouble {
			remaining := cmd[i:]
			if strings.HasPrefix(remaining, "&&") || strings.HasPrefix(remaining, "||") {
				if strings.TrimSpace(current) != "" {
					segments = append(segments, strings.TrimSpace(current))
				}
				current = ""
				i++
				continue
			}
			if c == '|' || c == ';' {
				if strings.TrimSpace(current) != "" {
					segments = append(segments, strings.TrimSpace(current))
				}
				current = ""
				continue
			}
			current += string(c)
		} else {
			current += string(c)
		}
	}

	if strings.TrimSpace(current) != "" {
		segments = append(segments, strings.TrimSpace(current))
	}

	return segments
}

// extractBase gets the base command name from a segment, stripping path
// prefixes and env command wrappers.
func extractBase(cmd string) string {
	cmd = strings.TrimSpace(cmd)

	if strings.HasPrefix(cmd, "env ") {
		words := strings.Fields(cmd)
		for i, w := range words[1:] {
			if !strings.Contains(w, "=") {
				return words[i+1]
			}
		}
	}

	words := strings.Fields(cmd)
	if len(words) == 0 {
		return ""
	}

	base := words[0]
	if idx := strings.LastIndex(base, "/"); idx != -1 {
		base = base[idx+1:]
	}

	return base
}
