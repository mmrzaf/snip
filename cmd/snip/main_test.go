package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mmrzaf/snip/internal/app"
	"github.com/mmrzaf/snip/internal/config"
)

func TestRunCommandAcceptsDashModifierWithFollowingFlags(t *testing.T) {
	root := t.TempDir()
	cfg := config.Default()
	cfg.Root = root
	cfg.DefaultProfile = "full"
	cfg.Ignore.UseGitignore = false
	cfg.Slices = map[string]config.SliceConfig{
		"docs":  {Include: []string{"README.md"}, Priority: 10},
		"tests": {Include: []string{"**/*_test.go"}, Priority: 20},
	}
	cfg.Profiles = map[string]config.Profile{
		"full": {Enable: []string{"docs"}},
	}

	cfgPath := filepath.Join(root, ".snip.yaml")
	if err := config.Write(cfgPath, cfg); err != nil {
		t.Fatalf("config.Write: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("docs\n"), 0o644); err != nil {
		t.Fatalf("write README.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "app_test.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write app_test.go: %v", err)
	}

	outPath := filepath.Join(root, "bundle.md")

	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })

	os.Args = []string{
		"snip",
		"run",
		"full",
		"-docs",
		"+tests",
		"--max-chars",
		"200000",
		"--out",
		outPath,
		"--quiet",
		"--config",
		cfgPath,
	}

	if code := run(); code != app.ExitOK {
		t.Fatalf("run() code=%d want=%d", code, app.ExitOK)
	}

	b, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read bundle: %v", err)
	}
	out := string(b)
	if strings.Contains(out, "<<<FILE:README.md>>>") {
		t.Fatalf("docs file block should be disabled by -docs:\n%s", out)
	}
	if !strings.Contains(out, "<<<FILE:app_test.go>>>") {
		t.Fatalf("test file block should be enabled by +tests:\n%s", out)
	}
}
