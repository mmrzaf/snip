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

// Parse extracts file blocks from markdown-like text using a header template such as
// "===== FILE: {path} =====". It is intentionally strict and fails on ambiguity.
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
	var fenceChar byte
	var fenceMinCount int

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
			if isFenceClose(line, fenceChar, fenceMinCount) {
				inFence = false
				fenceChar = 0
				fenceMinCount = 0
			}
			continue
		}

		// If we see an arbitrary opening fence, enter fence mode.
		// This prevents matching headers inside unrelated code blocks.
		if ch, n, okFence := parseFenceOpen(line); okFence {
			inFence = true
			fenceChar = ch
			fenceMinCount = n
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

		// Scan forward for the next opening fence, allowing metadata/noise lines.
		var (
			foundOpen     bool
			openLineNo    int
			contentStart  int
			closeLineNo   int
			contentEnd    int
			blockFenceChr byte
			blockMinCount int
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

			if ch, n, okFence := parseFenceOpen(l2); okFence {
				foundOpen = true
				openLineNo = ln
				blockFenceChr = ch
				blockMinCount = n
				contentStart = next2
				j = next2
				break
			}
			j = next2
		}
		if !foundOpen {
			return nil, invalidf("header at line %d for %q has no code fence", lineNo, path)
		}

		// Scan until closing fence.
		for {
			l3, next3, ok3 := readLine(src, j)
			if !ok3 {
				break
			}
			ln++
			if isFenceClose(l3, blockFenceChr, blockMinCount) {
				closeLineNo = ln
				contentEnd = j
				j = next3
				break
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

func parseFenceOpen(line string) (char byte, count int, ok bool) {
	t := strings.TrimSpace(line)
	if len(t) < 3 {
		return 0, 0, false
	}
	c := t[0]
	if c != '`' && c != '~' {
		return 0, 0, false
	}
	n := 0
	for n < len(t) && t[n] == c {
		n++
	}
	if n < 3 {
		return 0, 0, false
	}
	// Any suffix is allowed (language/info string).
	return c, n, true
}

func isFenceClose(line string, char byte, minCount int) bool {
	t := strings.TrimSpace(line)
	if len(t) < minCount {
		return false
	}
	for i := 0; i < len(t); i++ {
		if t[i] != char {
			return false
		}
	}
	return true
}
