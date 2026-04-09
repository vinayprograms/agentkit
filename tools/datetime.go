package tools

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type dateTool struct{}

// Datetime returns a tool that gets the current date and time.
func Datetime() Tool { return &dateTool{} }

func (t *dateTool) Name() string { return "datetime" }
func (t *dateTool) Description() string {
	return "Get the current date and time. Replaces the Unix 'date' command. Supports format specifiers like date's +FORMAT (e.g., '+%Y-%m-%d', '+%H:%M:%S', '+%A'). Without a format, returns ISO 8601 datetime, date, time, day of week, and Unix timestamp."
}
func (t *dateTool) Parameters() map[string]Param {
	return map[string]Param{
		"format": {
			Type:        StringParam,
			Description: "Format string using date's +FORMAT syntax: %Y=year, %m=month, %d=day, %H=hour, %M=minute, %S=second, %A=weekday, %B=month name, %Z=timezone, %s=unix epoch, %F=%Y-%m-%d, %T=%H:%M:%S, %R=%H:%M. Prefix with + like date command.",
			Required:    false,
		},
		"timezone": {
			Type:        StringParam,
			Description: "IANA timezone (e.g., 'America/New_York', 'UTC'). Defaults to system local time.",
			Required:    false,
		},
	}
}

// dateFormatSpecifiers maps date(1) format codes to Go time format tokens.
var dateFormatSpecifiers = map[byte]string{
	'Y': "2006",
	'm': "01",
	'd': "02",
	'H': "15",
	'M': "04",
	'S': "05",
	'A': "Monday",
	'a': "Mon",
	'B': "January",
	'b': "Jan",
	'p': "PM",
	'Z': "MST",
	'z': "-0700",
	'F': "2006-01-02",
	'T': "15:04:05",
	'R': "15:04",
	'c': "Mon Jan _2 15:04:05 2006",
	'r': "03:04:05 PM",
	'n': "\n",
	't': "\t",
	'%': "%",
}

func convertDateFormat(format string) string {
	var result strings.Builder
	i := 0
	for i < len(format) {
		if format[i] == '%' && i+1 < len(format) {
			next := format[i+1]
			if next == 's' {
				// Special: %s = unix epoch, handled separately
				result.WriteString("{UNIX}")
				i += 2
				continue
			}
			if goFmt, ok := dateFormatSpecifiers[next]; ok {
				result.WriteString(goFmt)
			} else {
				result.WriteByte('%')
				result.WriteByte(next)
			}
			i += 2
		} else {
			result.WriteByte(format[i])
			i++
		}
	}
	return result.String()
}

func (t *dateTool) Execute(ctx context.Context, args Args) (string, error) {
	now := time.Now()

	if tz := args.StringOr("timezone", ""); tz != "" {
		loc, err := time.LoadLocation(tz)
		if err != nil {
			return "", fmt.Errorf("invalid timezone %q: %w", tz, err)
		}
		now = now.In(loc)
	}

	format := args.StringOr("format", "")
	if format != "" {
		// Strip leading + like date command
		format = strings.TrimPrefix(format, "+")
		goFmt := convertDateFormat(format)
		result := now.Format(goFmt)
		// Handle %s (unix epoch) which can't be a Go format
		result = strings.ReplaceAll(result, "{UNIX}", fmt.Sprintf("%d", now.Unix()))
		return result, nil
	}

	return strings.Join([]string{
		"datetime: " + now.Format(time.RFC3339),
		"date: " + now.Format("2006-01-02"),
		"time: " + now.Format("15:04:05"),
		"day: " + now.Weekday().String(),
		fmt.Sprintf("unix: %d", now.Unix()),
		"timezone: " + now.Location().String(),
	}, "\n"), nil
}
