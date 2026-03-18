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
	if cfg.AutoNameLines != 20 {
		t.Errorf("expected AutoNameLines default 20, got %d", cfg.AutoNameLines)
	}
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
				HiddenSessions: []string{"abc123", "def456"},
				ClaudeFlags:    []string{"--dangerously-skip-permissions", "--verbose"},
				AutoNameLines:  30,
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

			assertStringSliceEqual(t, "HiddenSessions", tc.cfg.HiddenSessions, got.HiddenSessions)
			assertStringSliceEqual(t, "ClaudeFlags", tc.cfg.ClaudeFlags, got.ClaudeFlags)
			if tc.cfg.AutoNameLines != got.AutoNameLines {
				t.Errorf("AutoNameLines: want %d, got %d", tc.cfg.AutoNameLines, got.AutoNameLines)
			}
		})
	}
}

func TestConfigLoadsOldFileGracefully(t *testing.T) {
	// Simulate an old config file without auto_name_lines
	dir := t.TempDir()
	oldConfig := `hidden_sessions = ["abc"]
claude_flags = ["--verbose"]
tmux_session_name = "ccs"
activity_lines = 5
`
	path := dir + "/config.toml"
	os.WriteFile(path, []byte(oldConfig), 0644)

	var cfg types.Config
	if err := toml.Unmarshal([]byte(oldConfig), &cfg); err != nil {
		t.Fatalf("Unmarshal old config failed: %v", err)
	}
	applyDefaults(&cfg)

	if cfg.AutoNameLines != 20 {
		t.Errorf("expected AutoNameLines default 20, got %d", cfg.AutoNameLines)
	}
	if cfg.TmuxSessionName != "ccs" {
		t.Errorf("expected tmux_session_name 'ccs', got %q", cfg.TmuxSessionName)
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
