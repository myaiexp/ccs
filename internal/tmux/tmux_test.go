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

func TestParseActiveWindowID(t *testing.T) {
	output := "0 @1\n1 @3\n0 @5\n"
	id, err := parseActiveWindowID(output)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "@3" {
		t.Errorf("expected @3, got %s", id)
	}
}

func TestParseActiveWindowID_NoActive(t *testing.T) {
	output := "0 @1\n0 @2\n"
	_, err := parseActiveWindowID(output)
	if err == nil {
		t.Error("expected error when no active window")
	}
}

func TestParseActiveWindowID_Empty(t *testing.T) {
	_, err := parseActiveWindowID("")
	if err == nil {
		t.Error("expected error for empty output")
	}
}

func TestParseAllPanes(t *testing.T) {
	output := "ccs @1 1234\nccs @2 5678\nother @5 9999\n"
	result, err := parseAllPanes(output)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(result))
	}
	if result["ccs"]["@1"] != 1234 {
		t.Errorf("expected ccs/@1 PID 1234, got %d", result["ccs"]["@1"])
	}
	if result["ccs"]["@2"] != 5678 {
		t.Errorf("expected ccs/@2 PID 5678, got %d", result["ccs"]["@2"])
	}
	if result["other"]["@5"] != 9999 {
		t.Errorf("expected other/@5 PID 9999, got %d", result["other"]["@5"])
	}
}

func TestParseAllPanes_Empty(t *testing.T) {
	result, err := parseAllPanes("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty result, got %d entries", len(result))
	}
}

func TestParseAllPanes_MalformedLines(t *testing.T) {
	output := "ccs @1 1234\nbadline\nccs @2 notanumber\n"
	result, err := parseAllPanes(output)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Only the valid first line should be parsed
	if len(result["ccs"]) != 1 {
		t.Errorf("expected 1 valid entry, got %d", len(result["ccs"]))
	}
	if result["ccs"]["@1"] != 1234 {
		t.Errorf("expected PID 1234, got %d", result["ccs"]["@1"])
	}
}
