package util

import (
	"os"
	"path/filepath"
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
