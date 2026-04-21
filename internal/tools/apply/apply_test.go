package apply

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParse_BasicSingleBlock(t *testing.T) {
	input := `===== FILE: src/main.go =====
` + "```go" + `
package main

func main() {}
` + "```" + `
`
	blocks, err := Parse(input, "===== FILE: {path} =====")
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	if blocks[0].Path != "src/main.go" {
		t.Errorf("path = %q, want src/main.go", blocks[0].Path)
	}
	content := string(blocks[0].Content)
	if !strings.Contains(content, "package main") {
		t.Errorf("content missing package line:\n%s", content)
	}
	if strings.Contains(content, "```") {
		t.Errorf("content contains fence characters:\n%s", content)
	}
}

func TestParse_MultipleBlocks(t *testing.T) {
	input := `### ` + "`file1.txt`" + `
` + "```" + `
content1
` + "```" + `
Some text in between.
### ` + "`file2.txt`" + `
` + "```" + `
content2
` + "```" + `
`
	blocks, err := Parse(input, "### `{path}`")
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(blocks))
	}
	if blocks[0].Path != "file1.txt" {
		t.Errorf("first path = %q", blocks[0].Path)
	}
	if blocks[1].Path != "file2.txt" {
		t.Errorf("second path = %q", blocks[1].Path)
	}
	if string(blocks[0].Content) != "content1\n" {
		t.Errorf("content1 = %q", string(blocks[0].Content))
	}
	if string(blocks[1].Content) != "content2\n" {
		t.Errorf("content2 = %q", string(blocks[1].Content))
	}
}

func TestParse_NestedFencesSameCount(t *testing.T) {
	// Outer fence ```md, inner fence ```bash, inner close ```.
	// The outer opening fence is NOT part of the content.
	// Content should contain two triple-backtick sequences: inner open and inner close.
	input := "### `./docs/deploy.md`\n" +
		"```md\n" +
		"# Deployment\n\n" +
		"```bash\n" +
		"alembic upgrade head\n" +
		"```\n\n" +
		"More text after inner fence.\n" +
		"```\n"
	blocks, err := Parse(input, "### `{path}`")
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	content := string(blocks[0].Content)
	if !strings.Contains(content, "```bash") {
		t.Errorf("content missing inner opening fence:\n%s", content)
	}
	if !strings.Contains(content, "alembic upgrade head") {
		t.Errorf("content missing inner content:\n%s", content)
	}
	if !strings.Contains(content, "More text after inner fence") {
		t.Errorf("content missing text after inner fence:\n%s", content)
	}
	// Expect exactly two triple-backtick fences in the extracted content:
	// the inner opening "```bash" and the inner closing "```".
	if got := strings.Count(content, "```"); got != 2 {
		t.Errorf("unexpected number of backtick fences in content: %d\n%s", got, content)
	}
}

func TestParse_NestedFencesMixedChars(t *testing.T) {
	// Outer uses tildes, inner uses backticks.
	input := "FILE: outer.txt\n" +
		"~~~md\n" +
		"# Title\n\n" +
		"```bash\n" +
		"echo hello\n" +
		"```\n\n" +
		"End of content.\n" +
		"~~~\n"
	blocks, err := Parse(input, "FILE: {path}")
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	content := string(blocks[0].Content)
	if !strings.Contains(content, "```bash") {
		t.Errorf("content missing inner backtick fence")
	}
	if !strings.Contains(content, "echo hello") {
		t.Errorf("content missing inner command")
	}
	if !strings.Contains(content, "End of content") {
		t.Errorf("content missing trailing text")
	}
}

func TestParse_DeeplyNestedFences(t *testing.T) {
	// Three levels of nesting.
	input := "==> file.txt\n" +
		"````\n" +
		"Level 1 content\n\n" +
		"```bash\n" +
		"Level 2 content\n\n" +
		"~~~\n" +
		"Level 3 content\n" +
		"~~~\n\n" +
		"End level 2\n" +
		"```\n\n" +
		"End level 1\n" +
		"````\n"
	blocks, err := Parse(input, "==> {path}")
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	content := string(blocks[0].Content)
	if !strings.Contains(content, "Level 1 content") {
		t.Errorf("missing level 1")
	}
	if !strings.Contains(content, "Level 2 content") {
		t.Errorf("missing level 2")
	}
	if !strings.Contains(content, "Level 3 content") {
		t.Errorf("missing level 3")
	}
	if strings.Contains(content, "````") {
		t.Errorf("content contains outer fence chars")
	}
}

func TestParse_HeaderInsideFencedBlockIgnored(t *testing.T) {
	// A header line inside an arbitrary code block should not start a new file block.
	input := "Some text before.\n\n" +
		"```\n" +
		"This is an arbitrary code block.\n" +
		"FILE: ignored.txt\n" +
		"It should not be parsed.\n" +
		"```\n\n" +
		"FILE: real.txt\n" +
		"```\n" +
		"real content\n" +
		"```\n"
	blocks, err := Parse(input, "FILE: {path}")
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	if blocks[0].Path != "real.txt" {
		t.Errorf("expected real.txt, got %q", blocks[0].Path)
	}
	if string(blocks[0].Content) != "real content\n" {
		t.Errorf("unexpected content: %q", string(blocks[0].Content))
	}
}

func TestParse_EmptyPathError(t *testing.T) {
	input := "FILE: \n" +
		"```\n" +
		"content\n" +
		"```\n"
	_, err := Parse(input, "FILE: {path}")
	if err == nil {
		t.Fatal("expected error for empty path, got nil")
	}
	if !IsKind(err, KindInvalidInput) {
		t.Errorf("expected KindInvalidInput, got %v", err)
	}
	if !strings.Contains(err.Error(), "empty path") {
		t.Errorf("error does not mention empty path: %v", err)
	}
}

func TestParse_DuplicatePathError(t *testing.T) {
	input := "FILE: dup.txt\n" +
		"```\nfirst\n```\n" +
		"FILE: dup.txt\n" +
		"```\nsecond\n```\n"
	_, err := Parse(input, "FILE: {path}")
	if err == nil {
		t.Fatal("expected duplicate path error")
	}
	if !strings.Contains(err.Error(), "ambiguous duplicate") {
		t.Errorf("error missing duplicate wording: %v", err)
	}
}

func TestParse_NoFenceAfterHeaderError(t *testing.T) {
	input := "FILE: missing.txt\nSome text but no fence.\n"
	_, err := Parse(input, "FILE: {path}")
	if err == nil {
		t.Fatal("expected error for missing fence")
	}
	if !strings.Contains(err.Error(), "no code fence") {
		t.Errorf("error missing 'no code fence': %v", err)
	}
}

func TestParse_UnclosedFenceError(t *testing.T) {
	input := "FILE: unclosed.txt\n```\ncontent but no closing fence\n"
	_, err := Parse(input, "FILE: {path}")
	if err == nil {
		t.Fatal("expected unclosed fence error")
	}
	if !strings.Contains(err.Error(), "unclosed code fence") {
		t.Errorf("error missing 'unclosed code fence': %v", err)
	}
}

func TestParse_AmbiguousHeaderBeforeFenceError(t *testing.T) {
	input := "FILE: first.txt\nFILE: second.txt\n```\ncontent\n```\n"
	_, err := Parse(input, "FILE: {path}")
	if err == nil {
		t.Fatal("expected ambiguity error")
	}
	if !strings.Contains(err.Error(), "has no code fence before next header") {
		t.Errorf("error missing expected phrase: %v", err)
	}
}

func TestParse_NoFileBlocksError(t *testing.T) {
	input := "Just some text without any header."
	_, err := Parse(input, "FILE: {path}")
	if err == nil {
		t.Fatal("expected no file blocks error")
	}
	if !strings.Contains(err.Error(), "no file blocks detected") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParse_InvalidHeaderTemplate(t *testing.T) {
	_, err := Parse("", "")
	if err == nil {
		t.Fatal("expected error for empty template")
	}
	_, err = Parse("", "no placeholder")
	if err == nil {
		t.Fatal("expected error for missing {path}")
	}
	_, err = Parse("", "{path} and {path}")
	if err == nil {
		t.Fatal("expected error for multiple {path}")
	}
}

func TestApply_PlanAndDryRun(t *testing.T) {
	dir := t.TempDir()
	blocks := []Block{
		{Path: "a.txt", Content: []byte("hello\n")},
		{Path: "sub/b.txt", Content: []byte("world\n")},
	}
	opts := Options{
		Root:       dir,
		FileHeader: "ignore",
		Write:      false,
		Force:      false,
	}
	res, err := Apply(blocks, opts)
	if err != nil {
		t.Fatalf("Apply dry-run failed: %v", err)
	}
	if !res.DryRun {
		t.Error("expected DryRun=true")
	}
	if res.Wrote != 0 {
		t.Errorf("Wrote = %d, want 0", res.Wrote)
	}
	if len(res.Files) != 2 {
		t.Fatalf("expected 2 planned files, got %d", len(res.Files))
	}
	for _, pf := range res.Files {
		if pf.Exists {
			t.Errorf("file %s should not exist yet", pf.RelPath)
		}
	}
	_, err = os.Stat(filepath.Join(dir, "a.txt"))
	if !errors.Is(err, os.ErrNotExist) {
		t.Error("file should not exist after dry-run")
	}
}

func TestApply_WriteFiles(t *testing.T) {
	dir := t.TempDir()
	blocks := []Block{
		{Path: "a.txt", Content: []byte("hello\n")},
		{Path: "sub/b.txt", Content: []byte("world\n")},
	}
	opts := Options{
		Root:       dir,
		FileHeader: "ignore",
		Write:      true,
		Force:      false,
	}
	res, err := Apply(blocks, opts)
	if err != nil {
		t.Fatalf("Apply write failed: %v", err)
	}
	if res.DryRun {
		t.Error("expected DryRun=false")
	}
	if res.Wrote != 2 {
		t.Errorf("Wrote = %d, want 2", res.Wrote)
	}
	data, err := os.ReadFile(filepath.Join(dir, "a.txt"))
	if err != nil {
		t.Fatalf("read a.txt: %v", err)
	}
	if string(data) != "hello\n" {
		t.Errorf("a.txt content = %q", string(data))
	}
	data, err = os.ReadFile(filepath.Join(dir, "sub", "b.txt"))
	if err != nil {
		t.Fatalf("read b.txt: %v", err)
	}
	if string(data) != "world\n" {
		t.Errorf("b.txt content = %q", string(data))
	}
}

func TestApply_ExistingFileWithoutForceFails(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "exists.txt")
	if err := os.WriteFile(existing, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	blocks := []Block{
		{Path: "exists.txt", Content: []byte("new")},
	}
	opts := Options{
		Root:       dir,
		FileHeader: "ignore",
		Write:      true,
		Force:      false,
	}
	_, err := Apply(blocks, opts)
	if err == nil {
		t.Fatal("expected error due to existing file without --force")
	}
	if !strings.Contains(err.Error(), "target exists") {
		t.Errorf("error should mention target exists: %v", err)
	}
}

func TestApply_OverwriteWithForce(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "exists.txt")
	if err := os.WriteFile(existing, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	blocks := []Block{
		{Path: "exists.txt", Content: []byte("new")},
	}
	opts := Options{
		Root:       dir,
		FileHeader: "ignore",
		Write:      true,
		Force:      true,
	}
	res, err := Apply(blocks, opts)
	if err != nil {
		t.Fatalf("Apply with force failed: %v", err)
	}
	if res.Wrote != 1 {
		t.Errorf("Wrote = %d, want 1", res.Wrote)
	}
	data, err := os.ReadFile(existing)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new" {
		t.Errorf("content not overwritten: got %q", string(data))
	}
	if !res.Files[0].Overwrite {
		t.Error("Overwrite flag not set")
	}
}

func TestApply_PathTraversalRejected(t *testing.T) {
	dir := t.TempDir()
	blocks := []Block{
		{Path: "../escape.txt", Content: []byte("bad")},
	}
	opts := Options{Root: dir, FileHeader: "ignore"}
	_, err := Apply(blocks, opts)
	if err == nil {
		t.Fatal("expected error for path traversal")
	}
	if !strings.Contains(err.Error(), "escapes root") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestApply_DuplicateTargetAfterResolve(t *testing.T) {
	dir := t.TempDir()
	blocks := []Block{
		{Path: "foo/../bar.txt", Content: []byte("a")},
		{Path: "bar.txt", Content: []byte("b")},
	}
	opts := Options{Root: dir, FileHeader: "ignore"}
	_, err := Apply(blocks, opts)
	if err == nil {
		t.Fatal("expected duplicate target error")
	}
	if !strings.Contains(err.Error(), "ambiguous duplicate target path") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestApply_TargetIsDirectoryFails(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "sub")
	if err := os.Mkdir(subdir, 0o755); err != nil {
		t.Fatal(err)
	}
	blocks := []Block{
		{Path: "sub", Content: []byte("content")},
	}
	opts := Options{Root: dir, FileHeader: "ignore"}
	_, err := Apply(blocks, opts)
	if err == nil {
		t.Fatal("expected directory target error")
	}
	if !strings.Contains(err.Error(), "is a directory") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestApply_EmptyBlocksError(t *testing.T) {
	_, err := Apply([]Block{}, Options{})
	if err == nil {
		t.Fatal("expected error for empty blocks")
	}
}

func TestIsKind(t *testing.T) {
	err := invalidf("test")
	if !IsKind(err, KindInvalidInput) {
		t.Error("IsKind should return true for matching kind")
	}
	if IsKind(err, KindIO) {
		t.Error("IsKind should return false for different kind")
	}
	if IsKind(nil, KindInvalidInput) {
		t.Error("IsKind(nil) should be false")
	}
	plain := errors.New("plain")
	if IsKind(plain, KindInvalidInput) {
		t.Error("plain error should not match")
	}
}
