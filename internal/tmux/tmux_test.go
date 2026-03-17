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
