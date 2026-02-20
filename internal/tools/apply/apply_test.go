package apply

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseSnipStyleHeaderTemplate(t *testing.T) {
	t.Parallel()

	in := strings.Join([]string{
		"# noise",
		"",
		"===== FILE: a/b.go =====",
		"lines: 1",
		"bytes: 5",
		"",
		"```go",
		"package p",
		"```",
		"",
		"===== FILE: README.md =====",
		"```md",
		"# hi",
		"```",
		"",
	}, "\n")

	got, err := Parse(in, "===== FILE: {path} =====")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len=%d want=2", len(got))
	}
	if got[0].Path != "a/b.go" {
		t.Fatalf("path0=%q", got[0].Path)
	}
	if string(got[0].Content) != "package p\n" {
		t.Fatalf("content0=%q", string(got[0].Content))
	}
	if got[1].Path != "README.md" {
		t.Fatalf("path1=%q", got[1].Path)
	}
	if string(got[1].Content) != "# hi\n" {
		t.Fatalf("content1=%q", string(got[1].Content))
	}
}

func TestParseRejectsDuplicateHeaders(t *testing.T) {
	t.Parallel()

	in := strings.Join([]string{
		"===== FILE: x.txt =====",
		"```",
		"a",
		"```",
		"===== FILE: x.txt =====",
		"```",
		"b",
		"```",
	}, "\n")

	_, err := Parse(in, "===== FILE: {path} =====")
	if err == nil {
		t.Fatalf("expected error")
	}
	if !IsKind(err, KindInvalidInput) {
		t.Fatalf("wrong kind: %v", err)
	}
}

func TestApplyDryRunAndWrite(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	inputPath := filepath.Join(root, "ai.txt")
	input := strings.Join([]string{
		"===== FILE: dir/one.txt =====",
		"```txt",
		"one",
		"```",
		"===== FILE: two.txt =====",
		"```",
		"two",
		"```",
		"",
	}, "\n")
	if err := os.WriteFile(inputPath, []byte(input), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	dry, err := Run(inputPath, Options{
		Root:       root,
		FileHeader: "===== FILE: {path} =====",
	})
	if err != nil {
		t.Fatalf("Run dry: %v", err)
	}
	if !dry.DryRun || dry.Wrote != 0 || len(dry.Files) != 2 {
		t.Fatalf("dry result unexpected: %+v", dry)
	}
	if _, err := os.Stat(filepath.Join(root, "dir", "one.txt")); !os.IsNotExist(err) {
		t.Fatalf("file should not exist in dry-run; err=%v", err)
	}

	wet, err := Run(inputPath, Options{
		Root:       root,
		FileHeader: "===== FILE: {path} =====",
		Write:      true,
	})
	if err != nil {
		t.Fatalf("Run write: %v", err)
	}
	if wet.DryRun || wet.Wrote != 2 {
		t.Fatalf("write result unexpected: %+v", wet)
	}
	b, err := os.ReadFile(filepath.Join(root, "dir", "one.txt"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(b) != "one\n" {
		t.Fatalf("one.txt=%q", string(b))
	}
}

func TestApplyRejectsEscapingRoot(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	_, err := Apply([]Block{
		{Path: "../oops.txt", Content: []byte("x")},
	}, Options{Root: root})
	if err == nil {
		t.Fatalf("expected error")
	}
	if !IsKind(err, KindInvalidInput) {
		t.Fatalf("wrong kind: %v", err)
	}
}

func TestApplyRejectsOverwriteUnlessForced(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	target := filepath.Join(root, "x.txt")
	if err := os.WriteFile(target, []byte("old"), 0o644); err != nil {
		t.Fatalf("WriteFile old: %v", err)
	}

	_, err := Apply([]Block{{Path: "x.txt", Content: []byte("new")}}, Options{Root: root})
	if err == nil {
		t.Fatalf("expected overwrite error")
	}
	if !IsKind(err, KindInvalidInput) {
		t.Fatalf("wrong kind: %v", err)
	}

	res, err := Apply([]Block{{Path: "x.txt", Content: []byte("new")}}, Options{
		Root:  root,
		Force: true,
		Write: true,
	})
	if err != nil {
		t.Fatalf("Apply force write: %v", err)
	}
	if res.Wrote != 1 {
		t.Fatalf("Wrote=%d", res.Wrote)
	}
	b, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(b) != "new" {
		t.Fatalf("x.txt=%q", string(b))
	}
}
