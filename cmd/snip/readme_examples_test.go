package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mmrzaf/snip/internal/app"
)

func TestReadmeQuickStartExamples(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "main_test.go"), []byte("package main\n\nfunc TestMain(t *testing.T) {}\n"), 0o644); err != nil {
		t.Fatalf("write main_test.go: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("# temp repo\n"), 0o644); err != nil {
		t.Fatalf("write README.md: %v", err)
	}

	code, stdout, stderr := runCLI(t, root, "snip", "init")
	if code != app.ExitOK {
		t.Fatalf("init code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "created ") {
		t.Fatalf("init stdout missing created path: %q", stdout)
	}
	if _, err := os.Stat(filepath.Join(root, ".snip.yaml")); err != nil {
		t.Fatalf("missing .snip.yaml: %v", err)
	}

	checkOK := func(name string, args ...string) string {
		t.Helper()
		code, stdout, stderr := runCLI(t, root, args...)
		if code != app.ExitOK {
			t.Fatalf("%s code=%d stdout=%q stderr=%q", name, code, stdout, stderr)
		}
		return stdout
	}

	_ = checkOK("snip", "snip")
	if _, err := os.Stat(filepath.Join(root, ".snip", "last.md")); err != nil {
		t.Fatalf("missing generated latest bundle: %v", err)
	}

	stdout = checkOK("snip run api", "snip", "run", "api")
	if !strings.Contains(stdout, ".snip/") {
		t.Fatalf("run stdout missing output path: %q", stdout)
	}

	stdout = checkOK("snip run api +tests -docs", "snip", "run", "api", "+tests", "-docs")
	if !strings.Contains(stdout, ".snip/") {
		t.Fatalf("run modifiers stdout missing output path: %q", stdout)
	}

	stdout = checkOK("snip ls api", "snip", "ls", "api")
	if !strings.Contains(stdout, "Enabled slices:") || !strings.Contains(stdout, "main.go") {
		t.Fatalf("ls stdout missing expected content:\n%s", stdout)
	}

	stdout = checkOK("snip doctor", "snip", "doctor")
	if !strings.Contains(stdout, "snip doctor") || !strings.Contains(stdout, "profile: api") {
		t.Fatalf("doctor stdout missing expected content:\n%s", stdout)
	}

	stdout = checkOK("snip explain main.go", "snip", "explain", "main.go")
	if !strings.Contains(stdout, "snip explain") || !strings.Contains(stdout, "included: true") {
		t.Fatalf("explain stdout missing expected content:\n%s", stdout)
	}

	stdout = checkOK("snip apply .snip/last.md", "snip", "apply", ".snip/last.md")
	if !strings.Contains(stdout, "DRY-RUN plan:") {
		t.Fatalf("apply stdout missing dry-run plan:\n%s", stdout)
	}
}
