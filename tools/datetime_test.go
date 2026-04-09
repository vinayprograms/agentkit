package tools

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestDatetimeDefault(t *testing.T) {
	tool := Datetime()
	args, err := Validate(tool.Parameters(), nil)
	if err != nil {
		t.Fatal(err)
	}

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}

	for _, field := range []string{"datetime:", "date:", "time:", "day:", "unix:", "timezone:"} {
		if !strings.Contains(result, field) {
			t.Errorf("expected output to contain %q", field)
		}
	}
}

func TestDatetimeCustomFormat(t *testing.T) {
	tool := Datetime()
	args, err := Validate(tool.Parameters(), map[string]any{"format": "+%Y-%m-%d"})
	if err != nil {
		t.Fatal(err)
	}

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}

	// Should match today's date in YYYY-MM-DD format
	expected := time.Now().Format("2006-01-02")
	if result != expected {
		t.Errorf("got %q, want %q", result, expected)
	}
}

func TestDatetimeWithTimezone(t *testing.T) {
	tool := Datetime()
	args, err := Validate(tool.Parameters(), map[string]any{
		"format":   "+%Z",
		"timezone": "UTC",
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}

	if result != "UTC" {
		t.Errorf("got %q, want %q", result, "UTC")
	}
}

func TestDatetimeInvalidTimezone(t *testing.T) {
	tool := Datetime()
	args, err := Validate(tool.Parameters(), map[string]any{"timezone": "Invalid/Zone"})
	if err != nil {
		t.Fatal(err)
	}

	_, err = tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error for invalid timezone")
	}
}

func TestDatetime_UnixEpochFormat(t *testing.T) {
	tool := Datetime()
	args, _ := Validate(tool.Parameters(), map[string]any{"format": "+%s"})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	// Result should be a unix timestamp (all digits)
	for _, c := range result {
		if c < '0' || c > '9' {
			t.Errorf("expected numeric unix timestamp, got %q", result)
			break
		}
	}
}

func TestDatetime_AllFormatSpecifiers(t *testing.T) {
	tool := Datetime()
	// Test a format string with many specifiers
	args, _ := Validate(tool.Parameters(), map[string]any{
		"format":   "+%Y %m %d %H %M %S %A %a %B %b %p %Z %z %F %T %R %c %r %n %t %%",
		"timezone": "UTC",
	})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	// Should contain a literal %
	if !strings.Contains(result, "%") {
		t.Errorf("expected literal %% in result, got %q", result)
	}
	// Should contain a newline (from %n)
	if !strings.Contains(result, "\n") {
		t.Errorf("expected newline from %%n in result, got %q", result)
	}
	// Should contain a tab (from %t)
	if !strings.Contains(result, "\t") {
		t.Errorf("expected tab from %%t in result, got %q", result)
	}
}

func TestDatetime_UnknownSpecifier(t *testing.T) {
	tool := Datetime()
	// %Q is not a known specifier
	args, _ := Validate(tool.Parameters(), map[string]any{"format": "+%Q"})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	// Unknown specifier should be kept as-is
	if result != "%Q" {
		t.Errorf("expected '%%Q' for unknown specifier, got %q", result)
	}
}

func TestConvertDateFormat_TrailingPercent(t *testing.T) {
	// A trailing % without a following char
	result := convertDateFormat("hello%")
	if result != "hello%" {
		t.Errorf("expected 'hello%%', got %q", result)
	}
}
