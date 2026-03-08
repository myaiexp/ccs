package tmux

import (
	"testing"
)

func TestInTmux_Set(t *testing.T) {
	t.Setenv("TMUX", "/tmp/tmux-1000/default,12345,0")
	if !InTmux() {
		t.Error("expected InTmux() to return true when $TMUX is set")
	}
}

func TestInTmux_Unset(t *testing.T) {
	t.Setenv("TMUX", "")
	if InTmux() {
		t.Error("expected InTmux() to return false when $TMUX is empty")
	}
}

func TestNewWindow_CommandConstruction(t *testing.T) {
	args := newWindowArgs("claude", "/home/user/project", []string{"claude", "--resume", "abc123"})
	expected := []string{
		"new-window", "-P", "-F", "#{window_id}",
		"-n", "claude",
		"-c", "/home/user/project",
		"claude", "--resume", "abc123",
	}
	if len(args) != len(expected) {
		t.Fatalf("arg count mismatch: got %d, want %d\n  got:  %v\n  want: %v", len(args), len(expected), args, expected)
	}
	for i := range expected {
		if args[i] != expected[i] {
			t.Errorf("arg[%d] = %q, want %q", i, args[i], expected[i])
		}
	}
}

func TestSelectWindow_CommandConstruction(t *testing.T) {
	args := selectWindowArgs("@3")
	expected := []string{"select-window", "-t", "@3"}
	if len(args) != len(expected) {
		t.Fatalf("arg count mismatch: got %d, want %d", len(args), len(expected))
	}
	for i := range expected {
		if args[i] != expected[i] {
			t.Errorf("arg[%d] = %q, want %q", i, args[i], expected[i])
		}
	}
}
