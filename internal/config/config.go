package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/codeyogico/a2a-probe/internal/model"
)

func configPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".a2a", "config.json")
}

// Load reads ~/.a2a/config.json, returning an empty config on any error.
func Load() model.CliConfig {
	data, err := os.ReadFile(configPath())
	if err != nil {
		return model.CliConfig{}
	}
	var cfg model.CliConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return model.CliConfig{}
	}
	return cfg
}

// Save writes the config to ~/.a2a/config.json.
func Save(cfg model.CliConfig) error {
	p := configPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o644)
}

// ResolveServerURL returns the full URL for a name-or-URL argument.
func ResolveServerURL(nameOrURL string) (string, error) {
	if strings.HasPrefix(nameOrURL, "http://") ||
		strings.HasPrefix(nameOrURL, "https://") ||
		strings.HasPrefix(nameOrURL, "ws://") ||
		strings.HasPrefix(nameOrURL, "wss://") {
		return nameOrURL, nil
	}
	cfg := Load()
	if cfg.Servers == nil {
		return "", fmt.Errorf("unknown server %q — add it with: a2a-probe config add %s <url>", nameOrURL, nameOrURL)
	}
	srv, ok := cfg.Servers[nameOrURL]
	if !ok {
		return "", fmt.Errorf("unknown server %q — add it with: a2a-probe config add %s <url>", nameOrURL, nameOrURL)
	}
	return srv.URL, nil
}
