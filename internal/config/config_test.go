package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/codeyogico/a2a-probe/internal/model"
)

// overrideHome temporarily sets HOME so configPath() returns a test-local path.
func overrideHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	return dir
}

func TestLoadReturnsEmptyOnMissing(t *testing.T) {
	overrideHome(t)
	cfg := Load()
	if len(cfg.Servers) != 0 {
		t.Errorf("want empty servers, got %v", cfg.Servers)
	}
}

func TestLoadReturnsEmptyOnBadJSON(t *testing.T) {
	dir := overrideHome(t)
	p := filepath.Join(dir, ".a2a", "config.json")
	os.MkdirAll(filepath.Dir(p), 0o755)
	os.WriteFile(p, []byte("not-json"), 0o644)
	cfg := Load()
	if len(cfg.Servers) != 0 {
		t.Errorf("want empty on bad JSON, got %v", cfg.Servers)
	}
}

func TestSaveAndLoad(t *testing.T) {
	overrideHome(t)
	cfg := model.CliConfig{
		Servers: map[string]model.ServerConfig{
			"local": {URL: "http://localhost:8000", Transport: "http"},
		},
	}
	if err := Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got := Load()
	if len(got.Servers) != 1 {
		t.Fatalf("servers: want 1, got %d", len(got.Servers))
	}
	if got.Servers["local"].URL != "http://localhost:8000" {
		t.Errorf("url mismatch")
	}
}

func TestSaveCreatesDirectory(t *testing.T) {
	dir := overrideHome(t)
	cfg := model.CliConfig{}
	if err := Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".a2a", "config.json")); err != nil {
		t.Errorf("config file should exist: %v", err)
	}
}

func TestResolveServerURL_DirectURL(t *testing.T) {
	overrideHome(t)
	for _, u := range []string{
		"http://localhost:8000",
		"https://agent.example.com",
		"ws://localhost:9000",
		"wss://secure.example.com",
	} {
		got, err := ResolveServerURL(u)
		if err != nil {
			t.Errorf("ResolveServerURL(%q): unexpected error: %v", u, err)
		}
		if got != u {
			t.Errorf("ResolveServerURL(%q): want %q, got %q", u, u, got)
		}
	}
}

func TestResolveServerURL_NamedAlias(t *testing.T) {
	overrideHome(t)
	cfg := model.CliConfig{
		Servers: map[string]model.ServerConfig{
			"myagent": {URL: "http://localhost:9999", Transport: "sse"},
		},
	}
	Save(cfg)

	got, err := ResolveServerURL("myagent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "http://localhost:9999" {
		t.Errorf("want http://localhost:9999, got %s", got)
	}
}

func TestResolveServerURL_UnknownAlias(t *testing.T) {
	overrideHome(t)
	_, err := ResolveServerURL("unknown")
	if err == nil {
		t.Fatal("expected error for unknown alias")
	}
}

func TestResolveServerURL_EmptyConfig(t *testing.T) {
	overrideHome(t)
	_, err := ResolveServerURL("someserver")
	if err == nil {
		t.Fatal("expected error when no config exists")
	}
}
