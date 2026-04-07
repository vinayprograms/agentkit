package contentguard

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	base64Pattern      = regexp.MustCompile(`^[A-Za-z0-9+/]+={0,2}$`)
	base64URLPattern   = regexp.MustCompile(`^[A-Za-z0-9\-_]+={0,2}$`)
	hexPattern         = regexp.MustCompile(`^[0-9a-fA-F]+$`)
	urlEncodingPattern = regexp.MustCompile(`(%[0-9A-Fa-f]{2}){3,}`)
)

// hasEncodedContent returns true if the content contains encoded payloads
// (URL encoding, base64, base64url, or hex).
func hasEncodedContent(content string) bool {
	if urlEncodingPattern.MatchString(content) {
		return true
	}

	for _, seg := range extractAlphanumericSegments(content, 50) {
		if shannonEntropy([]byte(seg)) < entropyThreshold {
			continue
		}
		if isValidBase64(seg) || isValidBase64URL(seg) || isValidHex(seg) {
			return true
		}
	}

	return false
}

func isValidBase64(s string) bool {
	if len(s) < 4 || len(s)%4 != 0 {
		return false
	}
	return base64Pattern.MatchString(s)
}

func isValidBase64URL(s string) bool {
	if len(s) < 4 {
		return false
	}
	return base64URLPattern.MatchString(s)
}

func isValidHex(s string) bool {
	if len(s)%2 != 0 || len(s) < 16 {
		return false
	}
	return hexPattern.MatchString(s)
}

// extractAlphanumericSegments finds contiguous base64-compatible strings of at least minLen.
func extractAlphanumericSegments(content string, minLen int) []string {
	var segments []string
	var current []byte

	for i := 0; i < len(content); i++ {
		c := content[i]
		if isBase64Char(c) {
			current = append(current, c)
		} else {
			if len(current) >= minLen {
				segments = append(segments, string(current))
			}
			current = current[:0]
		}
	}

	if len(current) >= minLen {
		segments = append(segments, string(current))
	}

	return segments
}

func isBase64Char(c byte) bool {
	return (c >= 'A' && c <= 'Z') ||
		(c >= 'a' && c <= 'z') ||
		(c >= '0' && c <= '9') ||
		c == '+' || c == '/' || c == '=' ||
		c == '-' || c == '_'
}

// namedPattern pairs a name with a compiled regex for injection detection.
type namedPattern struct {
	name    string
	pattern *regexp.Regexp
}

var builtinPatterns = []namedPattern{
	// Instruction override attempts
	{"ignore_previous", regexp.MustCompile(`(?i)ignore\s+(previous|above|prior|all)\s+(instruction|directive|rule)`)},
	{"new_instruction", regexp.MustCompile(`(?i)new\s+(instruction|directive|task|policy)`)},
	{"forget_previous", regexp.MustCompile(`(?i)forget\s+(previous|everything|all)`)},

	// Superseding attempts (immutability violation)
	{"update_policy", regexp.MustCompile(`(?i)(update|change|modify)\s+(the\s+)?(policy|rule|instruction)`)},
	{"override", regexp.MustCompile(`(?i)override`)},
	{"supersede", regexp.MustCompile(`(?i)supersede`)},
	{"disregard_previous", regexp.MustCompile(`(?i)disregard\s+(previous|above|prior)`)},

	// System prompt extraction
	{"reveal_prompt", regexp.MustCompile(`(?i)(reveal|show|print|display)\s+(your\s+)?(system\s+)?prompt`)},
	{"what_instructions", regexp.MustCompile(`(?i)what\s+(are\s+)?(your\s+)?instructions`)},

	// Code execution attempts
	{"execute_code", regexp.MustCompile(`(?i)(execute|run|call|eval)\s*\(`)},
	{"curl_pipe_bash", regexp.MustCompile(`(?i)curl\s+.+\|\s*(ba)?sh`)},
	{"wget_pipe_bash", regexp.MustCompile(`(?i)wget\s+.+\|\s*(ba)?sh`)},
}

var builtinKeywords = []string{
	"api_key",
	"api-key",
	"apikey",
	"password",
	"secret",
	"credential",
	"private_key",
	"access_token",
}

// buildPatterns merges builtins with custom "name:regex" strings.
func buildPatterns(custom []string) ([]namedPattern, error) {
	patterns := make([]namedPattern, len(builtinPatterns))
	copy(patterns, builtinPatterns)

	for _, p := range custom {
		parts := strings.SplitN(p, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid pattern format %q (expected name:regex)", p)
		}
		compiled, err := regexp.Compile(parts[1])
		if err != nil {
			return nil, fmt.Errorf("invalid regex in pattern %q: %w", parts[0], err)
		}
		patterns = append(patterns, namedPattern{name: parts[0], pattern: compiled})
	}
	return patterns, nil
}

// buildKeywords merges builtins with custom keywords.
func buildKeywords(custom []string) []string {
	keywords := make([]string, len(builtinKeywords))
	copy(keywords, builtinKeywords)
	return append(keywords, custom...)
}

// detectSuspiciousPatterns returns the names of matched injection patterns.
func (g *Guard) detectSuspiciousPatterns(content string) []string {
	var names []string
	for _, p := range g.patterns {
		if p.pattern.MatchString(content) {
			names = append(names, p.name)
		}
	}
	return names
}

// detectSensitiveKeywords returns matched sensitive keywords.
func (g *Guard) detectSensitiveKeywords(content string) []string {
	var matched []string
	lower := strings.ToLower(content)
	for _, kw := range g.keywords {
		if strings.Contains(lower, strings.ToLower(kw)) {
			matched = append(matched, kw)
		}
	}
	return matched
}
