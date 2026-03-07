package config

import (
	"ccs/internal/types"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

func Load() (*types.Config, error) {
	cfg := &types.Config{}

	home, err := os.UserHomeDir()
	if err != nil {
		return cfg, nil
	}

	path := filepath.Join(home, ".config", "ccs", "config.toml")
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, nil // missing config is fine
	}

	if err := toml.Unmarshal(data, cfg); err != nil {
		return cfg, err
	}

	return cfg, nil
}
