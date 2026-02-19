package budget

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mmrzaf/snip/internal/selector"
)

func TestPerFileTruncationByLines(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	p := filepath.Join(dir, "a.txt")
	content := "l1\nl2\nl3\nl4\nl5\n"
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	b := &Builder{Limits: Limits{MaxChars: 100000, PerFileMaxLines: 3, PerFileMaxBytes: 1 << 20}}
	selected := selector.Selected{Included: []selector.File{{
		RelPath:         "a.txt",
		AbsPath:         p,
		Slices:          []string{"api"},
		PrimarySlice:    "api",
		PrimaryPriority: 10,
	}}}
	plan, err := b.BuildPlan(context.Background(), "p", []string{"api"}, selected)
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	if len(plan.Included) != 1 {
		t.Fatalf("included=%d", len(plan.Included))
	}
	fe := plan.Included[0]
	if !fe.Truncated {
		t.Fatalf("expected truncated")
	}
	if fe.OriginalLines != 5 {
		t.Fatalf("original lines=%d", fe.OriginalLines)
	}
	if fe.KeptLines != 3 {
		t.Fatalf("kept lines=%d", fe.KeptLines)
	}
	if !strings.Contains(fe.Content, "[TRUNCATED: original_lines=5 kept_lines=3]") {
		t.Fatalf("missing marker: %q", fe.Content)
	}
	if !strings.HasPrefix(fe.Content, "l1\nl2\nl3\n") {
		t.Fatalf("unexpected kept prefix: %q", fe.Content)
	}
}

func TestInvalidUTF8IsDropped(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	p := filepath.Join(dir, "bad.bin")
	// Invalid UTF-8 sequence.
	if err := os.WriteFile(p, []byte{0xff, 0xfe, 0xfd}, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	b := &Builder{Limits: Limits{MaxChars: 100000, PerFileMaxLines: 10, PerFileMaxBytes: 1 << 20}}
	selected := selector.Selected{Included: []selector.File{{
		RelPath:         "bad.bin",
		AbsPath:         p,
		Slices:          []string{"api"},
		PrimarySlice:    "api",
		PrimaryPriority: 10,
	}}}
	plan, err := b.BuildPlan(context.Background(), "p", []string{"api"}, selected)
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	if len(plan.Included) != 0 {
		t.Fatalf("included=%d", len(plan.Included))
	}
	if len(plan.Dropped) != 1 {
		t.Fatalf("dropped=%d", len(plan.Dropped))
	}
	if plan.Dropped[0].Reason != "invalid_utf8" {
		t.Fatalf("reason=%s", plan.Dropped[0].Reason)
	}
	if !plan.Partial {
		t.Fatalf("expected partial")
	}
}

func TestGlobalBudgetDropsLowPrioritySlice(t *testing.T) {
	t.Parallel()

	b := &Builder{Limits: Limits{MaxChars: 10, PerFileMaxLines: 10, PerFileMaxBytes: 1 << 20}}
	plan := Plan{
		Profile:       "p",
		EnabledSlices: []string{"api", "docs"},
		Included: []FileEntry{
			{RelPath: "a", AbsPath: "/x/a", Slices: []string{"api"}, PrimarySlice: "api", Priority: 100, Content: "aaaa"},
			{RelPath: "b", AbsPath: "/x/b", Slices: []string{"docs"}, PrimarySlice: "docs", Priority: 1, Content: "bbbb"},
		},
	}
	slicePriorities := map[string]int{"api": 100, "docs": 1}
	renderFn := func(p Plan) (string, error) {
		// Over budget unless only one file remains.
		if len(p.Included) == 2 {
			return "0123456789AB", nil
		}
		return "0123456789", nil
	}
	final, rendered, err := b.EnforceGlobalBudget(context.Background(), plan, slicePriorities, renderFn)
	if err != nil {
		t.Fatalf("EnforceGlobalBudget: %v", err)
	}
	if rendered != "0123456789" {
		t.Fatalf("rendered=%q", rendered)
	}
	if len(final.Included) != 1 {
		t.Fatalf("included=%d", len(final.Included))
	}
	if final.Included[0].PrimarySlice != "api" {
		t.Fatalf("kept=%s", final.Included[0].PrimarySlice)
	}
	if len(final.DroppedSlices) != 1 || final.DroppedSlices[0] != "docs" {
		t.Fatalf("droppedSlices=%v", final.DroppedSlices)
	}
	if !final.Partial {
		t.Fatalf("expected partial")
	}
}
