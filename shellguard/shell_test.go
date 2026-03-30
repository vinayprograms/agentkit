package shellguard

import "testing"

// Bash/Zsh shell tests

func TestBashShell_HasChainedCommands(t *testing.T) {
	sh := Bash()
	tests := []struct {
		cmd  string
		want bool
	}{
		{"ls -la", false},
		{"ls && echo done", true},
		{"ls || echo fail", true},
		{"ls; echo done", true},
		{"ls | grep foo", true},
		{"echo $(date)", true},
		{"echo ${HOME}", true},
		{"echo `date`", true},
		{"echo 'ls | grep'", false}, // quoted
		{`echo "ls && rm"`, false},  // quoted
	}
	for _, tt := range tests {
		t.Run(tt.cmd, func(t *testing.T) {
			got := sh.HasChainedCommands(tt.cmd)
			if got != tt.want {
				t.Errorf("HasChainedCommands(%q) = %v, want %v", tt.cmd, got, tt.want)
			}
		})
	}
}

func TestBashShell_SplitSegments(t *testing.T) {
	sh := Bash()
	tests := []struct {
		cmd      string
		expected []string
	}{
		{"ls | grep foo", []string{"ls", "grep foo"}},
		{"cd /tmp; ls", []string{"cd /tmp", "ls"}},
		{"make && make install", []string{"make", "make install"}},
		{"echo 'hello | world'", []string{"echo 'hello | world'"}},
	}
	for _, tt := range tests {
		t.Run(tt.cmd, func(t *testing.T) {
			got := sh.SplitSegments(tt.cmd)
			if len(got) != len(tt.expected) {
				t.Fatalf("SplitSegments(%q) = %v, want %v", tt.cmd, got, tt.expected)
			}
			for i := range got {
				if got[i] != tt.expected[i] {
					t.Errorf("segment[%d] = %q, want %q", i, got[i], tt.expected[i])
				}
			}
		})
	}
}

func TestBashShell_ExtractCommand(t *testing.T) {
	sh := Bash()
	tests := []struct {
		input, want string
	}{
		{"ls -la", "ls"},
		{"/usr/bin/curl http://x", "curl"},
		{"env VAR=1 python script.py", "python"},
		{"./local/tool arg", "tool"},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := sh.ExtractCommand(tt.input)
			if got != tt.want {
				t.Errorf("ExtractCommand(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// Fish shell tests

func TestFishShell_HasChainedCommands(t *testing.T) {
	sh := Fish()
	tests := []struct {
		cmd  string
		want bool
	}{
		{"ls -la", false},
		{"ls; echo done", true},
		{"ls | grep foo", true},
		{"echo (date)", true},
		{"echo `date`", true},
		// Fish doesn't use && or || — these are literal text
		{"ls -la", false},
	}
	for _, tt := range tests {
		t.Run(tt.cmd, func(t *testing.T) {
			got := sh.HasChainedCommands(tt.cmd)
			if got != tt.want {
				t.Errorf("HasChainedCommands(%q) = %v, want %v", tt.cmd, got, tt.want)
			}
		})
	}
}

func TestFishShell_SplitSegments(t *testing.T) {
	sh := Fish()
	result := sh.SplitSegments("ls; echo done")
	if len(result) != 2 || result[0] != "ls" || result[1] != "echo done" {
		t.Errorf("SplitSegments = %v, want [ls, echo done]", result)
	}
}

func TestFishShell_ExtractCommand(t *testing.T) {
	sh := Fish()
	tests := []struct {
		input, want string
	}{
		{"ls -la", "ls"},
		{"and echo done", "echo"},  // fish "; and" → "and echo done" after split
		{"or echo fail", "echo"},   // fish "; or" → "or echo fail" after split
		{"/usr/bin/curl x", "curl"},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := sh.ExtractCommand(tt.input)
			if got != tt.want {
				t.Errorf("ExtractCommand(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// POSIX shell tests

func TestPosixShell_HasChainedCommands(t *testing.T) {
	sh := Posix()
	tests := []struct {
		cmd  string
		want bool
	}{
		{"ls -la", false},
		{"ls && echo done", true},
		{"ls | grep foo", true},
		{"ls; echo done", true},
		{"echo `date`", true},
		// POSIX doesn't have $() or ${}
		{"echo $(date)", false},
		{"echo ${HOME}", false},
	}
	for _, tt := range tests {
		t.Run(tt.cmd, func(t *testing.T) {
			got := sh.HasChainedCommands(tt.cmd)
			if got != tt.want {
				t.Errorf("HasChainedCommands(%q) = %v, want %v", tt.cmd, got, tt.want)
			}
		})
	}
}

// Gate with different shells

func TestGate_FishShell(t *testing.T) {
	gate := New(Fish(), "/workspace", nil, nil, nil, "")
	// curl is banned regardless of shell
	allowed, _, _ := gate.Check(t.Context(), "curl http://evil.com")
	if allowed {
		t.Error("curl should be blocked in fish shell")
	}
	// Safe command
	allowed, _, _ = gate.Check(t.Context(), "ls -la")
	if !allowed {
		t.Error("ls should be allowed in fish shell")
	}
}

func TestGate_PosixShell(t *testing.T) {
	gate := New(Posix(), "/workspace", nil, nil, nil, "")
	// sudo in chain
	allowed, _, _ := gate.Check(t.Context(), "make && sudo make install")
	if allowed {
		t.Error("sudo in chain should be blocked in posix shell")
	}
}
