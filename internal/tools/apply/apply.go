package apply

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/mmrzaf/snip/internal/util"
)

// Kind classifies apply errors without coupling to CLI exit codes.
type Kind int

// Kind represents error categories for apply operations.
const (
	// KindInvalidInput indicates an invalid or malformed input.
	KindInvalidInput Kind = iota + 1
	// KindIO indicates an I/O-related failure.
	KindIO
)

// Error is a typed error for callers to map to their own policies/exit codes.
type Error struct {
	Kind Kind
	Err  error
}

func (e *Error) Error() string { return e.Err.Error() }
func (e *Error) Unwrap() error { return e.Err }

func invalidf(format string, args ...any) error {
	return &Error{Kind: KindInvalidInput, Err: fmt.Errorf(format, args...)}
}

func iof(err error, format string, args ...any) error {
	return &Error{Kind: KindIO, Err: fmt.Errorf(format+": %w", append(args, err)...)}
}

// IsKind reports whether err is an *Error of the given kind.
func IsKind(err error, kind Kind) bool {
	var e *Error
	if !errors.As(err, &e) {
		return false
	}
	return e.Kind == kind
}

// Options configures parsing + apply behavior.
type Options struct {
	Root       string
	FileHeader string // Required. Must contain exactly one {path} token.
	Write      bool   // Default false (dry-run).
	Force      bool   // Default false (no overwrite).
}

// Block is one parsed file payload.
type Block struct {
	Path    string // as declared in input (trimmed)
	Content []byte // exact bytes between opening and closing fence after newline normalization
}

// PlannedFile is a validated filesystem operation.
type PlannedFile struct {
	RelPath   string
	AbsPath   string
	Content   []byte
	Exists    bool
	Overwrite bool
}

// Result is the parsed + validated plan, with optional writes applied.
type Result struct {
	Files  []PlannedFile
	Wrote  int
	DryRun bool
}

// Run reads an input file (or stdin when inputPath == "-"), parses file/code blocks, validates paths
// against root, and optionally writes them.
func Run(inputPath string, opts Options) (Result, error) {
	text, err := readInput(inputPath)
	if err != nil {
		return Result{}, err
	}
	blocks, err := Parse(text, opts.FileHeader)
	if err != nil {
		return Result{}, err
	}
	return Apply(blocks, opts)
}

// fenceInfo holds the parsed properties of an opening fence.
type fenceInfo struct {
	char  byte
	count int
	line  string // original trimmed line (for potential future use)
}

// parseFenceOpen examines a line and returns fence info if it is a valid opening fence.
// It follows CommonMark: a line that begins with at least three backticks or tildes.
// The remainder of the line may contain an info string (e.g., language).
func parseFenceOpen(line string) (info fenceInfo, ok bool) {
	trimmed := strings.TrimSpace(line)
	if len(trimmed) < 3 {
		return fenceInfo{}, false
	}
	c := trimmed[0]
	if c != '`' && c != '~' {
		return fenceInfo{}, false
	}
	n := 0
	for n < len(trimmed) && trimmed[n] == c {
		n++
	}
	if n < 3 {
		return fenceInfo{}, false
	}
	return fenceInfo{char: c, count: n, line: trimmed}, true
}

// isFenceClose checks if a line is a valid closing fence for the given opening info.
// It follows CommonMark: the line must start with at least minCount fence chars
// (after stripping leading whitespace), and the remainder (after all fence chars)
// must consist only of whitespace (i.e., no info string).
func isFenceClose(line string, open fenceInfo) bool {
	leftTrimmed := strings.TrimLeft(line, " \t")
	if len(leftTrimmed) < open.count {
		return false
	}
	// Must start with the same fence char repeated at least open.count times.
	for i := 0; i < open.count; i++ {
		if leftTrimmed[i] != open.char {
			return false
		}
	}
	// Any additional characters beyond the minimum count must also be the fence char.
	i := open.count
	for i < len(leftTrimmed) && leftTrimmed[i] == open.char {
		i++
	}
	// The rest of the line (after stripping all fence chars) must be only whitespace.
	rest := leftTrimmed[i:]
	return strings.TrimSpace(rest) == ""
}

// countFenceChars returns the number of consecutive occurrences of char at the start
// of the trimmed line, or 0 if the line does not consist solely of that char + whitespace.
func countFenceChars(line string, char byte) int {
	trimmed := strings.TrimSpace(line)
	if len(trimmed) == 0 || trimmed[0] != char {
		return 0
	}
	n := 0
	for n < len(trimmed) && trimmed[n] == char {
		n++
	}
	// Ensure the rest is only whitespace (closing fence condition).
	if strings.TrimSpace(trimmed[n:]) != "" {
		return 0
	}
	return n
}

// Parse extracts file blocks from markdown-like text using a header template such as
// "===== FILE: {path} =====". It handles nested code fences correctly.
func Parse(input string, fileHeader string) ([]Block, error) {
	hm, err := compileHeaderMatcher(fileHeader)
	if err != nil {
		return nil, err
	}

	src := util.NormalizeNewlines(input)
	var blocks []Block
	seen := make(map[string]int)

	// Track whether we are inside a *non-apply* fenced block.
	// If we are, headers are ignored until the fence closes.
	inFence := false
	var fenceInfoCurrent fenceInfo

	i := 0
	lineNo := 0
	for {
		line, next, ok := readLine(src, i)
		if !ok {
			break
		}
		i = next
		lineNo++

		// If we're currently inside an arbitrary fence (not part of a file block),
		// ignore everything until it closes.
		if inFence {
			if isFenceClose(line, fenceInfoCurrent) {
				inFence = false
				fenceInfoCurrent = fenceInfo{}
			}
			continue
		}

		// If we see an arbitrary opening fence, enter fence mode.
		// This prevents matching headers inside unrelated code blocks.
		if info, ok := parseFenceOpen(line); ok {
			inFence = true
			fenceInfoCurrent = info
			continue
		}

		// Only now do we consider header matching.
		path, matched := hm.match(line)
		if !matched {
			continue
		}

		path = strings.TrimSpace(path)
		if path == "" {
			return nil, invalidf("empty path in header at line %d", lineNo)
		}
		if prev, dup := seen[path]; dup {
			return nil, invalidf("ambiguous duplicate file path %q (headers at lines %d and %d)", path, prev, lineNo)
		}
		seen[path] = lineNo

		// Scan forward for the next opening fence (the outer fence of the file block).
		var (
			foundOpen    bool
			openLineNo   int
			blockFence   fenceInfo
			contentStart int
		)

		j := i
		ln := lineNo
		for {
			l2, next2, ok2 := readLine(src, j)
			if !ok2 {
				break
			}
			ln++

			// A second header before the first fence is ambiguity => fail.
			if _, m2 := hm.match(l2); m2 {
				return nil, invalidf("header at line %d for %q has no code fence before next header at line %d", lineNo, path, ln)
			}

			if info, ok := parseFenceOpen(l2); ok {
				foundOpen = true
				openLineNo = ln
				blockFence = info
				contentStart = next2
				j = next2
				break
			}
			j = next2
		}
		if !foundOpen {
			return nil, invalidf("header at line %d for %q has no code fence", lineNo, path)
		}

		// Use a stack to track nested fences.
		// The outer block ends when the stack becomes empty.
		stack := []fenceInfo{blockFence}
		contentEnd := -1
		closeLineNo := 0

		for {
			l3, next3, ok3 := readLine(src, j)
			if !ok3 {
				break
			}
			ln++

			// IMPORTANT: If we are inside a fence (stack non-empty), we must first check
			// if this line closes the current top. Only if it does NOT close do we consider
			// it as a possible opening fence. This prevents the same line from being
			// misinterpreted as both an opening and a closing fence.
			if len(stack) > 0 {
				top := stack[len(stack)-1]
				if isFenceClose(l3, top) {
					// This line closes the top fence.
					stack = stack[:len(stack)-1]
					if len(stack) == 0 {
						// This is the closing fence of the outer block.
						closeLineNo = ln
						contentEnd = j
						j = next3
						break
					}
					// It was an inner closing fence; continue scanning.
					j = next3
					continue
				}
			}

			// Not a closing fence; check if it's an opening fence.
			if openInfo, okOpen := parseFenceOpen(l3); okOpen {
				stack = append(stack, openInfo)
			}
			j = next3
		}

		if closeLineNo == 0 {
			return nil, invalidf("unclosed code fence for %q (header line %d, fence line %d)", path, lineNo, openLineNo)
		}

		content := src[contentStart:contentEnd]
		blocks = append(blocks, Block{
			Path:    path,
			Content: []byte(content),
		})

		// Continue scanning after the closing fence.
		i = j
		lineNo = closeLineNo
	}

	if len(blocks) == 0 {
		return nil, invalidf("no file blocks detected")
	}
	return blocks, nil
}

// Apply validates paths, plans operations, and optionally writes files.
func Apply(blocks []Block, opts Options) (Result, error) {
	if len(blocks) == 0 {
		return Result{}, invalidf("no file blocks detected")
	}

	root, err := effectiveRoot(opts.Root)
	if err != nil {
		return Result{}, err
	}

	plan := make([]PlannedFile, 0, len(blocks))
	seenRel := make(map[string]int)
	for i, b := range blocks {
		rel, abs, err := resolveTarget(root, b.Path)
		if err != nil {
			return Result{}, err
		}
		if first, ok := seenRel[rel]; ok {
			return Result{}, invalidf("ambiguous duplicate target path %q (entries %d and %d)", rel, first+1, i+1)
		}
		seenRel[rel] = i

		st, statErr := os.Stat(abs)
		exists := statErr == nil
		if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
			return Result{}, iof(statErr, "stat %s", rel)
		}
		if exists && st.IsDir() {
			return Result{}, invalidf("target %q is a directory", rel)
		}
		if exists && !opts.Force {
			return Result{}, invalidf("target exists (use --force): %q", rel)
		}

		plan = append(plan, PlannedFile{
			RelPath:   rel,
			AbsPath:   abs,
			Content:   append([]byte(nil), b.Content...),
			Exists:    exists,
			Overwrite: exists && opts.Force,
		})
	}

	res := Result{
		Files:  plan,
		DryRun: !opts.Write,
	}
	if !opts.Write {
		return res, nil
	}

	for _, pf := range plan {
		if err := util.AtomicWriteFile(pf.AbsPath, pf.Content, 0o644); err != nil {
			return Result{}, iof(err, "write %s", pf.RelPath)
		}
		res.Wrote++
	}
	return res, nil
}

type headerMatcher struct {
	prefix string
	suffix string
}

func compileHeaderMatcher(tpl string) (headerMatcher, error) {
	tpl = util.NormalizeNewlines(tpl)
	tpl = strings.TrimSuffix(tpl, "\n")
	if strings.TrimSpace(tpl) == "" {
		return headerMatcher{}, invalidf("file header template is required (must contain {path})")
	}
	if strings.Count(tpl, "{path}") != 1 {
		return headerMatcher{}, invalidf("file header template must contain exactly one {path} token")
	}
	idx := strings.Index(tpl, "{path}")
	return headerMatcher{
		prefix: tpl[:idx],
		suffix: tpl[idx+len("{path}"):],
	}, nil
}

func (m headerMatcher) match(line string) (string, bool) {
	if !strings.HasPrefix(line, m.prefix) {
		return "", false
	}
	if !strings.HasSuffix(line, m.suffix) {
		return "", false
	}
	return line[len(m.prefix) : len(line)-len(m.suffix)], true
}

func readInput(path string) (string, error) {
	if path == "" {
		return "", invalidf("input file is required")
	}
	var b []byte
	var err error
	if path == "-" {
		b, err = io.ReadAll(os.Stdin)
		if err != nil {
			return "", iof(err, "read stdin")
		}
		return string(b), nil
	}
	b, err = os.ReadFile(path)
	if err != nil {
		return "", iof(err, "read input file %s", path)
	}
	return string(b), nil
}

func effectiveRoot(root string) (string, error) {
	if root == "" {
		root = "."
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", iof(err, "abs root")
	}
	st, err := os.Stat(abs)
	if err != nil {
		return "", iof(err, "stat root")
	}
	if !st.IsDir() {
		return "", invalidf("root is not a directory: %s", abs)
	}
	return abs, nil
}

func resolveTarget(rootAbs string, declared string) (rel string, abs string, err error) {
	p := strings.TrimSpace(declared)
	if p == "" {
		return "", "", invalidf("empty path")
	}
	if strings.ContainsRune(p, '\x00') {
		return "", "", invalidf("path contains NUL: %q", p)
	}

	clean := filepath.Clean(filepath.FromSlash(p))
	if clean == "." {
		return "", "", invalidf("invalid target path %q", p)
	}
	if filepath.IsAbs(clean) {
		return "", "", invalidf("absolute paths are not allowed: %q", p)
	}

	abs = filepath.Clean(filepath.Join(rootAbs, clean))
	relCheck, relErr := filepath.Rel(rootAbs, abs)
	if relErr != nil {
		return "", "", iof(relErr, "rel path")
	}
	if relCheck == ".." || strings.HasPrefix(relCheck, ".."+string(filepath.Separator)) {
		return "", "", invalidf("path escapes root: %q", p)
	}
	if relCheck == "." {
		return "", "", invalidf("invalid target path %q", p)
	}
	return filepath.ToSlash(relCheck), abs, nil
}

func readLine(s string, start int) (line string, next int, ok bool) {
	if start >= len(s) {
		return "", start, false
	}
	i := start
	for i < len(s) && s[i] != '\n' {
		i++
	}
	if i < len(s) {
		return s[start:i], i + 1, true
	}
	return s[start:i], i, true
}
