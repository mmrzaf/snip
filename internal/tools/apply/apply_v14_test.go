package apply

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseAutoDetectsDefaultHeaders(t *testing.T) {
	input := strings.Join([]string{
		"noise",
		"<<<FILE:pkg/x.go>>>",
		"```go",
		"package x",
		"```",
		"",
	}, "\n")

	blocks, err := ParseAuto(input)
	if err != nil {
		t.Fatalf("ParseAuto: %v", err)
	}
	if len(blocks) != 1 || blocks[0].Path != "pkg/x.go" {
		t.Fatalf("blocks=%#v", blocks)
	}
}

func TestDryRunExistingFileDoesNotError(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "pkg", "x.go")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := Apply([]Block{{Path: "pkg/x.go", Content: []byte("new\n")}}, Options{Root: root})
	if err != nil {
		t.Fatalf("Apply dry-run: %v", err)
	}
	if len(res.Files) != 1 || !res.Files[0].Blocked {
		t.Fatalf("expected blocked dry-run plan: %#v", res.Files)
	}
	if !strings.Contains(res.PlanSummary(), "1 blocked") {
		t.Fatalf("summary=%q", res.PlanSummary())
	}
	if !strings.Contains(res.VerbosePlan(), "BLOCKED") {
		t.Fatalf("verbose=%q", res.VerbosePlan())
	}
}

func TestWriteExistingFileRequiresForce(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "pkg", "x.go")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Apply([]Block{{Path: "pkg/x.go", Content: []byte("new\n")}}, Options{Root: root, Write: true})
	if err == nil {
		t.Fatalf("expected overwrite error")
	}
	if !IsKind(err, KindInvalidInput) {
		t.Fatalf("err kind=%v", err)
	}

	b, readErr := os.ReadFile(target)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(b) != "old\n" {
		t.Fatalf("content changed without force: %q", string(b))
	}
}
