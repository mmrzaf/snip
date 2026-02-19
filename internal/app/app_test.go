package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mmrzaf/snip/internal/config"
)

func TestWriteExplicitOutputRelativePath(t *testing.T) {
	dir := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	defer func() { _ = os.Chdir(oldwd) }()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}

	got, err := writeExplicitOutput("out/bundle.md", "hello\n")
	if err != nil {
		t.Fatalf("writeExplicitOutput: %v", err)
	}
	want := filepath.Join(dir, "out", "bundle.md")
	if got != want {
		t.Fatalf("path=%q want=%q", got, want)
	}
	b, err := os.ReadFile(got)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(b) != "hello\n" {
		t.Fatalf("content=%q", string(b))
	}
}

func TestWriteDefaultOutputUsesCounterAndLatest(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	cfg := config.Default()
	cfg.Output.Dir = ".snip-out"
	cfg.Output.Pattern = "bundle_{profile}_{counter}"
	cfg.Output.Latest = "latest.md"

	ts := time.Date(2026, 2, 19, 10, 30, 0, 0, time.UTC)
	out1, err := writeDefaultOutput(root, cfg, "api", "abc123", ts, "first")
	if err != nil {
		t.Fatalf("writeDefaultOutput #1: %v", err)
	}
	if !strings.HasSuffix(out1, "bundle_api_001.md") {
		t.Fatalf("out1=%q", out1)
	}

	out2, err := writeDefaultOutput(root, cfg, "api", "abc123", ts, "second")
	if err != nil {
		t.Fatalf("writeDefaultOutput #2: %v", err)
	}
	if !strings.HasSuffix(out2, "bundle_api_002.md") {
		t.Fatalf("out2=%q", out2)
	}

	latestPath := filepath.Join(root, ".snip-out", "latest.md")
	latest, err := os.ReadFile(latestPath)
	if err != nil {
		t.Fatalf("read latest: %v", err)
	}
	if string(latest) != "second" {
		t.Fatalf("latest content=%q want second", string(latest))
	}
}

func TestDoctorAndExplainIncludeUsefulDiagnostics(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	cfg := config.Default()
	cfg.Root = root
	cfg.DefaultProfile = "api"
	cfg.Ignore.UseGitignore = false
	cfg.Slices = map[string]config.SliceConfig{
		"api": {Include: []string{"**/*.txt", "README.md"}, Priority: 10},
	}
	cfg.Profiles = map[string]config.Profile{
		"api": {Enable: []string{"api"}},
	}

	cfgPath := filepath.Join(root, ".snip.yaml")
	if err := config.Write(cfgPath, cfg); err != nil {
		t.Fatalf("config.Write: %v", err)
	}

	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("readme"), 0o644); err != nil {
		t.Fatalf("write README.md: %v", err)
	}
	// Should be excluded by default sensitive pattern "**/*secret*".
	if err := os.WriteFile(filepath.Join(root, "app-secret.txt"), []byte("secret"), 0o644); err != nil {
		t.Fatalf("write app-secret.txt: %v", err)
	}

	docOut, err := Doctor(context.Background(), DoctorOptions{
		ConfigPath: cfgPath,
	})
	if err != nil {
		t.Fatalf("Doctor: %v", err)
	}
	if !strings.Contains(docOut, "enabled_slices: [api]") {
		t.Fatalf("Doctor output missing enabled slices:\n%s", docOut)
	}
	if !strings.Contains(docOut, "excluded_sensitive: 1") {
		t.Fatalf("Doctor output missing exclusion reason count:\n%s", docOut)
	}

	explainOut, err := Explain(context.Background(), ExplainOptions{
		ConfigPath: cfgPath,
		Path:       "app-secret.txt",
	})
	if err != nil {
		t.Fatalf("Explain: %v", err)
	}
	if !strings.Contains(explainOut, "reason: excluded_sensitive") {
		t.Fatalf("Explain output missing sensitive reason:\n%s", explainOut)
	}
	if !strings.Contains(explainOut, "included: false") {
		t.Fatalf("Explain output missing inclusion verdict:\n%s", explainOut)
	}
}

func TestRunWritesBundleForSimpleRepo(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	cfg := config.Default()
	cfg.Root = root
	cfg.DefaultProfile = "p"
	cfg.Ignore.UseGitignore = false
	cfg.Slices = map[string]config.SliceConfig{
		"code": {Include: []string{"**/*.go"}, Priority: 10},
	}
	cfg.Profiles = map[string]config.Profile{
		"p": {Enable: []string{"code"}},
	}

	cfgPath := filepath.Join(root, ".snip.yaml")
	if err := config.Write(cfgPath, cfg); err != nil {
		t.Fatalf("config.Write: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}

	outPath := filepath.Join(root, "bundle.md")
	res, err := Run(context.Background(), RunOptions{
		ConfigPath: cfgPath,
		Profile:    "p",
		Output:     outPath,
		Now: func() time.Time {
			return time.Date(2026, 2, 19, 10, 0, 0, 0, time.UTC)
		},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.OutputPath != outPath {
		t.Fatalf("OutputPath=%q want=%q", res.OutputPath, outPath)
	}
	if res.Partial || res.HardCut {
		t.Fatalf("unexpected partial result: %+v", res)
	}

	b, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read bundle: %v", err)
	}
	out := string(b)
	if !strings.Contains(out, "main.go") {
		t.Fatalf("bundle missing file block:\n%s", out)
	}
	if !strings.Contains(out, "enabled_slices: [code]") {
		t.Fatalf("bundle missing manifest:\n%s", out)
	}
}
