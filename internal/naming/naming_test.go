package naming

import (
	"testing"
)

func TestParseResponse_SKIP(t *testing.T) {
	if got := parseResponse("SKIP"); got != "" {
		t.Errorf("SKIP → %q, want empty", got)
	}
	if got := parseResponse("skip"); got != "" {
		t.Errorf("skip → %q, want empty", got)
	}
	if got := parseResponse("  SKIP  "); got != "" {
		t.Errorf("padded SKIP → %q, want empty", got)
	}
}

func TestParseResponse_Empty(t *testing.T) {
	if got := parseResponse(""); got != "" {
		t.Errorf("empty → %q, want empty", got)
	}
	if got := parseResponse("   \n  "); got != "" {
		t.Errorf("whitespace → %q, want empty", got)
	}
}

func TestParseResponse_Normal(t *testing.T) {
	if got := parseResponse("config-sync autopull setup"); got != "config-sync autopull setup" {
		t.Errorf("got %q", got)
	}
}

func TestParseResponse_Trimmed(t *testing.T) {
	if got := parseResponse("  auth middleware migration  "); got != "auth middleware migration" {
		t.Errorf("got %q", got)
	}
}

func TestParseResponse_MultilineFirstLineOnly(t *testing.T) {
	input := "first line name\nsecond line explanation\nthird line"
	if got := parseResponse(input); got != "first line name" {
		t.Errorf("got %q, want %q", got, "first line name")
	}
}

