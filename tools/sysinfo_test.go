package tools

import (
	"context"
	"strings"
	"testing"
)

func TestSysinfo(t *testing.T) {
	tool := Sysinfo()
	args, err := Validate(tool.Parameters(), nil)
	if err != nil {
		t.Fatal(err)
	}

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}

	if result == "" {
		t.Fatal("expected non-empty sysinfo")
	}

	for _, field := range []string{"os:", "arch:", "cpus:", "hostname:", "user:", "cwd:"} {
		if !strings.Contains(result, field) {
			t.Errorf("expected output to contain %q", field)
		}
	}
}
