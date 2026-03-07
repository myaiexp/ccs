package config

import (
	"ccs/internal/types"
	"os"
	"testing"

	"github.com/BurntSushi/toml"
)

func TestLoadReturnsDefaults(t *testing.T) {
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if cfg == nil {
		t.Fatal("Load() returned nil config")
	}
	// Even if no config file exists, cfg should be a valid zero-value struct
}

func TestLoadRealConfigIfExists(t *testing.T) {
	path := configPath()
	if path == "" {
		t.Skip("could not determine config path")
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skipf("no config file at %s", path)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error with existing config: %v", err)
	}
	if cfg == nil {
		t.Fatal("Load() returned nil config with existing config file")
	}
}

func TestTOMLRoundtrip(t *testing.T) {
	tests := []struct {
		name string
		cfg  types.Config
	}{
		{
			name: "empty config",
			cfg:  types.Config{},
		},
		{
			name: "all fields populated",
			cfg: types.Config{
				HiddenProjects: []string{"cloned", ".claude"},
				HiddenSessions: []string{"abc123", "def456"},
				ClaudeFlags:    []string{"--dangerously-skip-permissions", "--verbose"},
			},
		},
		{
			name: "only hidden_projects",
			cfg: types.Config{
				HiddenProjects: []string{"secret-project"},
			},
		},
		{
			name: "only claude_flags",
			cfg: types.Config{
				ClaudeFlags: []string{"--flag"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data, err := toml.Marshal(&tc.cfg)
			if err != nil {
				t.Fatalf("Marshal failed: %v", err)
			}

			var got types.Config
			if err := toml.Unmarshal(data, &got); err != nil {
				t.Fatalf("Unmarshal failed: %v", err)
			}

			assertStringSliceEqual(t, "HiddenProjects", tc.cfg.HiddenProjects, got.HiddenProjects)
			assertStringSliceEqual(t, "HiddenSessions", tc.cfg.HiddenSessions, got.HiddenSessions)
			assertStringSliceEqual(t, "ClaudeFlags", tc.cfg.ClaudeFlags, got.ClaudeFlags)
		})
	}
}

func assertStringSliceEqual(t *testing.T, field string, want, got []string) {
	t.Helper()
	if len(want) != len(got) {
		t.Errorf("%s: length mismatch: want %d, got %d\n  want: %v\n  got:  %v", field, len(want), len(got), want, got)
		return
	}
	for i := range want {
		if want[i] != got[i] {
			t.Errorf("%s[%d]: want %q, got %q", field, i, want[i], got[i])
		}
	}
}
