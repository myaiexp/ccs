package tmux

import (
	"testing"
)

func TestParseBindingLine(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		wantKey string
		wantCmd string
	}{
		{
			name:    "select-window binding",
			line:    "bind-key -T prefix 1 select-window -t :1",
			wantKey: "1",
			wantCmd: "select-window -t :1",
		},
		{
			name:    "space next-layout",
			line:    "bind-key -T prefix Space next-layout",
			wantKey: "Space",
			wantCmd: "next-layout",
		},
		{
			name:    "select-window 2",
			line:    "bind-key -T prefix 2 select-window -t :2",
			wantKey: "2",
			wantCmd: "select-window -t :2",
		},
		{
			name:    "multi-arg command",
			line:    "bind-key -T prefix x confirm-before -p 'kill-window #W? (y/n)' kill-window",
			wantKey: "x",
			wantCmd: "confirm-before -p 'kill-window #W? (y/n)' kill-window",
		},
		{
			name:    "too few fields",
			line:    "bind-key -T prefix",
			wantKey: "",
			wantCmd: "",
		},
		{
			name:    "wrong table",
			line:    "bind-key -T root C-b send-prefix",
			wantKey: "",
			wantCmd: "",
		},
		{
			name:    "empty line",
			line:    "",
			wantKey: "",
			wantCmd: "",
		},
		{
			name:    "not bind-key",
			line:    "unbind-key -T prefix 1",
			wantKey: "",
			wantCmd: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			key, cmd := parseBindingLine(tc.line)
			if key != tc.wantKey {
				t.Errorf("key: got %q, want %q", key, tc.wantKey)
			}
			if cmd != tc.wantCmd {
				t.Errorf("cmd: got %q, want %q", cmd, tc.wantCmd)
			}
		})
	}
}

func TestParseBindings(t *testing.T) {
	lines := []string{
		"bind-key -T prefix 0 select-window -t :0",
		"bind-key -T prefix 1 select-window -t :1",
		"bind-key -T prefix 2 select-window -t :2",
		"bind-key -T prefix Space next-layout",
		"bind-key -T prefix c new-window",
		"bind-key -T prefix n next-window",
	}

	saved := parseBindings(lines)
	if saved.Space != "next-layout" {
		t.Errorf("Space: got %q, want %q", saved.Space, "next-layout")
	}
	if saved.One != "select-window -t :1" {
		t.Errorf("One: got %q, want %q", saved.One, "select-window -t :1")
	}
	if saved.Two != "select-window -t :2" {
		t.Errorf("Two: got %q, want %q", saved.Two, "select-window -t :2")
	}
}

func TestParseBindings_MissingKeys(t *testing.T) {
	lines := []string{
		"bind-key -T prefix c new-window",
		"bind-key -T prefix n next-window",
	}

	saved := parseBindings(lines)
	if saved.Space != "" {
		t.Errorf("Space should be empty, got %q", saved.Space)
	}
	if saved.One != "" {
		t.Errorf("One should be empty, got %q", saved.One)
	}
	if saved.Two != "" {
		t.Errorf("Two should be empty, got %q", saved.Two)
	}
}

func TestParseBindings_Empty(t *testing.T) {
	saved := parseBindings(nil)
	if saved.Space != "" || saved.One != "" || saved.Two != "" {
		t.Error("expected all empty for nil input")
	}
}

func TestBuildIfShellArgs(t *testing.T) {
	args := buildIfShellArgs(
		"Space",
		"tmux show -wv @ccs-managed 2>/dev/null",
		"select-window -t :0",
		"next-layout",
	)

	expected := []string{
		"bind-key", "-T", "prefix", "Space",
		"if-shell", "tmux show -wv @ccs-managed 2>/dev/null",
		"select-window -t :0", "next-layout",
	}

	if len(args) != len(expected) {
		t.Fatalf("len: got %d, want %d", len(args), len(expected))
	}
	for i := range expected {
		if args[i] != expected[i] {
			t.Errorf("args[%d]: got %q, want %q", i, args[i], expected[i])
		}
	}
}

func TestBuildIfShellArgs_NoFallback(t *testing.T) {
	// When there's no original binding, we use display-message '' as fallback
	args := buildIfShellArgs(
		"1",
		"tmux show -wv @ccs-managed 2>/dev/null",
		"previous-window",
		"display-message ''",
	)

	if args[7] != "display-message ''" {
		t.Errorf("fallback: got %q, want %q", args[7], "display-message ''")
	}
}
