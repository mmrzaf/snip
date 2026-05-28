package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mmrzaf/snip/internal/app"
	"github.com/mmrzaf/snip/internal/config"
)

func TestRunPrintsErrorToStderr(t *testing.T) {
	root := t.TempDir()

	code, _, stderr := runCLI(t, root, "snip", "--config", filepath.Join(root, "missing.yaml"), "run", "api")
	if code != app.ExitUsage {
		t.Fatalf("code=%d want=%d stderr=%q", code, app.ExitUsage, stderr)
	}
	if !strings.Contains(stderr, "error:") || !strings.Contains(stderr, "read config") {
		t.Fatalf("stderr missing useful error: %q", stderr)
	}
}

func TestRootCommandAcceptsDashModifiers(t *testing.T) {
	root, cfgPath := writeModifierFixture(t)

	outPath := filepath.Join(root, ".snip", "last.md")
	code, _, stderr := runCLI(t, root, "snip", "--config", cfgPath, "full", "-docs", "+tests")
	if code != app.ExitOK {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}

	b, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	out := string(b)
	if strings.Contains(out, "<<<FILE:README.md>>>") {
		t.Fatalf("docs file should be disabled by -docs:\n%s", out)
	}
	if !strings.Contains(out, "<<<FILE:app_test.go>>>") {
		t.Fatalf("test file should be enabled by +tests:\n%s", out)
	}
}

func TestLsDoctorExplainAcceptDashModifiers(t *testing.T) {
	root, cfgPath := writeModifierFixture(t)

	code, stdout, stderr := runCLI(t, root, "snip", "--config", cfgPath, "ls", "full", "-docs", "+tests")
	if code != app.ExitOK {
		t.Fatalf("ls code=%d stderr=%q", code, stderr)
	}
	if strings.Contains(stdout, "README.md") {
		t.Fatalf("ls should not include README.md:\n%s", stdout)
	}
	if !strings.Contains(stdout, "app_test.go") {
		t.Fatalf("ls should include app_test.go:\n%s", stdout)
	}

	code, stdout, stderr = runCLI(t, root, "snip", "--config", cfgPath, "doctor", "--profile", "full", "-docs", "+tests")
	if code != app.ExitOK {
		t.Fatalf("doctor code=%d stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, "enabled_slices: [tests]") {
		t.Fatalf("doctor missing enabled tests:\n%s", stdout)
	}

	code, stdout, stderr = runCLI(t, root, "snip", "--config", cfgPath, "explain", "app_test.go", "--profile", "full", "-docs", "+tests")
	if code != app.ExitOK {
		t.Fatalf("explain code=%d stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, "included: true") {
		t.Fatalf("explain should include app_test.go:\n%s", stdout)
	}
}

func TestApplyAutoDetectsSnipHeader(t *testing.T) {
	root := t.TempDir()
	input := strings.Join([]string{
		"<<<FILE:pkg/x.go>>>",
		"```go",
		"package x",
		"```",
		"",
	}, "\n")
	inputPath := filepath.Join(root, "bundle.md")
	if err := os.WriteFile(inputPath, []byte(input), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	code, stdout, stderr := runCLI(t, root, "snip", "--root", root, "apply", inputPath, "--write")
	if code != app.ExitOK {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, "Wrote 1 file") {
		t.Fatalf("stdout missing write summary: %q", stdout)
	}
	b, err := os.ReadFile(filepath.Join(root, "pkg", "x.go"))
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if string(b) != "package x\n" {
		t.Fatalf("content=%q", string(b))
	}
}

func TestVersionCommandPrintsCurrentVersion(t *testing.T) {
	root := t.TempDir()

	code, stdout, stderr := runCLI(t, root, "snip", "version")
	if code != app.ExitOK {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if strings.TrimSpace(stdout) != "1.4.0" {
		t.Fatalf("version=%q want 1.4.0", stdout)
	}
}

func TestRunPartialOutputStillPrintsPathAndWritesBundle(t *testing.T) {
	root := t.TempDir()
	cfg := config.Default()
	cfg.Root = root
	cfg.DefaultProfile = "api"
	cfg.Ignore.UseGitignore = false
	cfg.Budgets.MaxChars = 1000
	cfg.Slices = map[string]config.SliceConfig{
		"code": {Include: []string{"**/*.go"}, Priority: 10},
	}
	cfg.Profiles = map[string]config.Profile{
		"api": {Enable: []string{"code"}},
	}

	cfgPath := filepath.Join(root, ".snip.yaml")
	if err := config.Write(cfgPath, cfg); err != nil {
		t.Fatalf("config.Write: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n\n// "+strings.Repeat("x", 4000)+"\n"), 0o644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}

	outPath := filepath.Join(root, "bundle.md")
	code, stdout, stderr := runCLI(t, root, "snip", "--config", cfgPath, "run", "api", "--max-chars", "300", "--out", outPath)
	if code != app.ExitPartial {
		t.Fatalf("code=%d want=%d stdout=%q stderr=%q", code, app.ExitPartial, stdout, stderr)
	}
	if !strings.Contains(stdout, outPath) {
		t.Fatalf("stdout missing output path %q: %q", outPath, stdout)
	}
	if !strings.Contains(stderr, "warning: snapshot is partial") {
		t.Fatalf("stderr missing partial warning: %q", stderr)
	}
	if _, err := os.Stat(outPath); err != nil {
		t.Fatalf("bundle not written: %v", err)
	}
}

func TestApplyCommandReadsStdin(t *testing.T) {
	root := t.TempDir()
	input := strings.Join([]string{
		"<<<FILE:pkg/stdin.go>>>",
		"```go",
		"package stdin",
		"```",
		"",
	}, "\n")

	code, stdout, stderr := runCLIWithInput(t, root, input, "snip", "--root", root, "apply", "-", "--write")
	if code != app.ExitOK {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "Wrote 1 file") {
		t.Fatalf("stdout missing write summary: %q", stdout)
	}
	b, err := os.ReadFile(filepath.Join(root, "pkg", "stdin.go"))
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if string(b) != "package stdin\n" {
		t.Fatalf("content=%q", string(b))
	}
}

func writeModifierFixture(t *testing.T) (string, string) {
	t.Helper()

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

	return root, cfgPath
}

func runCLI(t *testing.T, cwd string, args ...string) (int, string, string) {
	t.Helper()
	return runCLIWithInput(t, cwd, "", args...)
}

func runCLIWithInput(t *testing.T, cwd string, stdin string, args ...string) (int, string, string) {
	t.Helper()

	oldArgs := os.Args
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	oldStdout := os.Stdout
	oldStderr := os.Stderr
	oldStdin := os.Stdin
	defer func() {
		os.Args = oldArgs
		os.Stdout = oldStdout
		os.Stderr = oldStderr
		os.Stdin = oldStdin
		if err := os.Chdir(oldWD); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	}()

	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	defer func() { _ = stdoutR.Close() }()
	defer func() { _ = stdoutW.Close() }()
	stderrR, stderrW, err := os.Pipe()
	if err != nil {
		t.Fatalf("stderr pipe: %v", err)
	}
	defer func() { _ = stderrR.Close() }()
	defer func() { _ = stderrW.Close() }()

	var stdinR *os.File
	if stdin != "" {
		var stdinW *os.File
		stdinR, stdinW, err = os.Pipe()
		if err != nil {
			t.Fatalf("stdin pipe: %v", err)
		}
		if _, err := stdinW.WriteString(stdin); err != nil {
			_ = stdinW.Close()
			_ = stdinR.Close()
			t.Fatalf("write stdin pipe: %v", err)
		}
		if err := stdinW.Close(); err != nil {
			_ = stdinR.Close()
			t.Fatalf("close stdin writer: %v", err)
		}
		defer func() { _ = stdinR.Close() }()
	}

	os.Args = append([]string(nil), args...)
	if err := os.Chdir(cwd); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	os.Stdout = stdoutW
	os.Stderr = stderrW
	if stdinR != nil {
		os.Stdin = stdinR
	}

	code := run()

	_ = stdoutW.Close()
	_ = stderrW.Close()

	var stdoutBuf, stderrBuf bytes.Buffer
	_, _ = io.Copy(&stdoutBuf, stdoutR)
	_, _ = io.Copy(&stderrBuf, stderrR)

	return code, stdoutBuf.String(), stderrBuf.String()
}
