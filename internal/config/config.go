package config

import (
	"ccs/internal/types"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

func configPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "ccs", "config.toml")
}

func Load() (*types.Config, error) {
	cfg := &types.Config{}

	path := configPath()
	if path == "" {
		return cfg, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		applyDefaults(cfg)
		return cfg, nil // missing config is fine
	}

	if err := toml.Unmarshal(data, cfg); err != nil {
		return cfg, err
	}

	applyDefaults(cfg)
	return cfg, nil
}

func applyDefaults(cfg *types.Config) {
	if cfg.TmuxSessionName == "" {
		cfg.TmuxSessionName = "ccs"
	}
	if cfg.ActivityLines <= 0 {
		cfg.ActivityLines = 5
	}
}

func Save(cfg *types.Config) error {
	path := configPath()
	if path == "" {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	return toml.NewEncoder(f).Encode(cfg)
}
