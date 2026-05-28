package util

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyPatternTokens(t *testing.T) {
	t.Parallel()

	got := ApplyPatternTokens("snip_{profile}_{ts}_{gitsha}_{repo}_{counter}.md", map[string]string{
		"profile": "api",
		"ts":      "20260101-000000",
		"gitsha":  "abc123",
		"repo":    "demo",
		"counter": "007",
	})
	want := "snip_api_20260101-000000_abc123_demo_007.md"
	if got != want {
		t.Fatalf("got=%q want=%q", got, want)
	}
}

func TestCollapseSeparators(t *testing.T) {
	t.Parallel()
	got := CollapseSeparators("snip__api--x..y")
	want := "snip_api-x.y"
	if got != want {
		t.Fatalf("got=%q want=%q", got, want)
	}
}

func TestNextCounter(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	c1, err := NextCounter(dir)
	if err != nil {
		t.Fatalf("NextCounter: %v", err)
	}
	if c1 != 1 {
		t.Fatalf("c1=%d", c1)
	}
	c2, err := NextCounter(dir)
	if err != nil {
		t.Fatalf("NextCounter2: %v", err)
	}
	if c2 != 2 {
		t.Fatalf("c2=%d", c2)
	}
	b, err := os.ReadFile(filepath.Join(dir, "counter"))
	if err != nil {
		t.Fatalf("read counter: %v", err)
	}
	if string(b) != "2\n" {
		t.Fatalf("file=%q", string(b))
	}
}

func TestAtomicWriteFileCreatesParentAndCleansTemp(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "out.txt")
	if err := AtomicWriteFile(path, []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("AtomicWriteFile: %v", err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(b) != "hello\n" {
		t.Fatalf("content=%q", string(b))
	}

	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".out.txt.tmp.") {
			t.Fatalf("stale temp file left behind: %s", entry.Name())
		}
	}
}
