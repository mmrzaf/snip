package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindConfigPathPrecedence(t *testing.T) {
	t.Setenv("SNIP_CONFIG", "/tmp/from-env.yaml")

	if got := FindConfigPath("/tmp/explicit.yaml"); got != "/tmp/explicit.yaml" {
		t.Fatalf("FindConfigPath explicit=%q", got)
	}
	if got := FindConfigPath(""); got != "/tmp/from-env.yaml" {
		t.Fatalf("FindConfigPath env=%q", got)
	}

	t.Setenv("SNIP_CONFIG", "")
	if got := FindConfigPath(""); got != ".snip.yaml" {
		t.Fatalf("FindConfigPath default=%q", got)
	}
}

func TestLoadMergesDefaultsAndInfersProfile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, ".snip.yaml")

	// Intentionally sparse to exercise merge/default behavior.
	yaml := `
name: demo
slices:
  docs:
    include: ["docs/**"]
    priority: 3
profiles:
  debug:
    enable: ["docs"]
`
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Version != 1 {
		t.Fatalf("Version=%d want 1", cfg.Version)
	}
	if cfg.Root != "." {
		t.Fatalf("Root=%q want '.'", cfg.Root)
	}
	if cfg.Output.Pattern == "" {
		t.Fatalf("Output.Pattern should be defaulted")
	}
	if cfg.Budgets.MaxChars <= 0 || cfg.Budgets.PerFileMaxLines <= 0 || cfg.Budgets.PerFileMaxBytes <= 0 {
		t.Fatalf("Budgets should be defaulted: %+v", cfg.Budgets)
	}
	if cfg.DefaultProfile != "debug" {
		t.Fatalf("DefaultProfile=%q want debug", cfg.DefaultProfile)
	}
}

func TestValidateRejectsInvalidConfig(t *testing.T) {
	t.Parallel()

	base := Default()
	base.DefaultProfile = "p"
	base.Slices = map[string]SliceConfig{
		"s": {Include: []string{"**/*.go"}, Priority: 1},
	}
	base.Profiles = map[string]Profile{
		"p": {Enable: []string{"s"}},
	}

	t.Run("invalid file block header", func(t *testing.T) {
		cfg := base
		cfg.Render.FileBlock.Header = "bad\nheader"
		err := Validate(cfg)
		if err == nil || !strings.Contains(err.Error(), "render.file_block.header") {
			t.Fatalf("Validate err=%v", err)
		}
	})

	t.Run("unknown slice in profile", func(t *testing.T) {
		cfg := base
		cfg.Profiles = map[string]Profile{
			"p": {Enable: []string{"missing"}},
		}
		err := Validate(cfg)
		if err == nil || !strings.Contains(err.Error(), `unknown slice "missing"`) {
			t.Fatalf("Validate err=%v", err)
		}
	})
}

func TestEffectiveRootAndWriteAndSymlinkGuard(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cfg := Default()
	cfg.DefaultProfile = "p"
	cfg.Slices = map[string]SliceConfig{
		"s": {Include: []string{"**/*"}, Priority: 1},
	}
	cfg.Profiles = map[string]Profile{
		"p": {Enable: []string{"s"}},
	}

	outPath := filepath.Join(dir, "nested", ".snip.yaml")
	if err := Write(outPath, cfg); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if _, err := os.Stat(outPath); err != nil {
		t.Fatalf("written config stat: %v", err)
	}

	abs, err := EffectiveRoot(cfg, dir)
	if err != nil {
		t.Fatalf("EffectiveRoot: %v", err)
	}
	if abs != dir {
		t.Fatalf("EffectiveRoot=%q want %q", abs, dir)
	}

	link := filepath.Join(dir, "link-root")
	if err := os.Symlink(dir, link); err != nil {
		t.Skipf("symlink unsupported in environment: %v", err)
	}
	if err := EnsureNoSymlinkRoot(link); err == nil {
		t.Fatalf("EnsureNoSymlinkRoot should reject symlink path")
	}
	if err := EnsureNoSymlinkRoot(dir); err != nil {
		t.Fatalf("EnsureNoSymlinkRoot real dir: %v", err)
	}
}
