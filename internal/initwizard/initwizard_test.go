package initwizard

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/mmrzaf/snip/internal/config"
)

func TestRunDefaultIsNonInteractiveAndUsesAPIProfile(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "go.mod"), "module example.com/app\n")
	mustWrite(t, filepath.Join(root, "main.go"), "package main\n")
	mustWrite(t, filepath.Join(root, "main_test.go"), "package main\n")
	mustWrite(t, filepath.Join(root, "README.md"), "# app\n")

	path, err := Run(Options{Root: root})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if path != filepath.Join(root, ".snip.yaml") {
		t.Fatalf("path=%q", path)
	}

	cfg := readGeneratedConfig(t, path)
	if cfg.DefaultProfile != "api" {
		t.Fatalf("default_profile=%q want api", cfg.DefaultProfile)
	}
	if cfg.Budgets.MaxChars != 200000 {
		t.Fatalf("budgets.max_chars=%d want 200000", cfg.Budgets.MaxChars)
	}
	if _, ok := cfg.Profiles["api"]; !ok {
		t.Fatalf("missing api profile: %#v", cfg.Profiles)
	}
	if _, ok := cfg.Profiles["full"]; !ok {
		t.Fatalf("missing full profile: %#v", cfg.Profiles)
	}
	if _, ok := cfg.Slices["tests"]; !ok {
		t.Fatalf("missing tests slice: %#v", cfg.Slices)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read raw config: %v", err)
	}
	text := string(raw)
	for _, bad := range []string{"max_chars: 0", "tree_depth: 0"} {
		if strings.Contains(text, bad) {
			t.Fatalf("generated config contains empty override %q:\n%s", bad, text)
		}
	}
}

func TestRunSkipsHeavyDirsDuringScan(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "go.mod"), "module example.com/app\n")
	mustWrite(t, filepath.Join(root, "main.go"), "package main\n")
	mustWrite(t, filepath.Join(root, "node_modules", "bad", "bad_test.go"), "package bad\n")
	mustWrite(t, filepath.Join(root, ".git", "secret.go"), "package bad\n")

	paths, err := collectRepoFiles(root)
	if err != nil {
		t.Fatalf("collectRepoFiles: %v", err)
	}
	for _, p := range paths {
		if p == "node_modules/bad/bad_test.go" || p == ".git/secret.go" {
			t.Fatalf("heavy dir file leaked into scan: %v", paths)
		}
	}
}

func TestRunExistingConfigReturnsSentinel(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, ".snip.yaml"), "version: 1\n")

	_, err := Run(Options{Root: root})
	if !errors.Is(err, ErrConfigExists) {
		t.Fatalf("err=%v want ErrConfigExists", err)
	}
}

func readGeneratedConfig(t *testing.T, path string) config.Config {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var cfg config.Config
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		t.Fatalf("yaml: %v", err)
	}
	return cfg
}

func mustWrite(t *testing.T, path string, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}
