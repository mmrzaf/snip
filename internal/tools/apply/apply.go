package apply

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/mmrzaf/snip/internal/util"
)

// Kind classifies apply errors.
type Kind int

const (
	// KindInvalidInput indicates invalid apply input.
	KindInvalidInput Kind = iota + 1
	// KindIO indicates a filesystem or stream I/O failure.
	KindIO
)

// Error is a typed apply error.
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

// IsKind reports whether err is an apply error of the given kind.
func IsKind(err error, kind Kind) bool {
	var e *Error
	if !errors.As(err, &e) {
		return false
	}
	return e.Kind == kind
}

// Options configures parsing and apply behavior.
type Options struct {
	Root       string
	FileHeader string // Optional. Auto-detected when empty.
	Write      bool
	Force      bool
}

// Result is the parsed and validated plan, with optional writes applied.
type Result struct {
	Files  []PlannedFile
	Wrote  int
	DryRun bool
}

// Run reads an input file (or stdin when inputPath == "-"), parses file/code blocks,
// validates paths against root, and optionally writes them.
func Run(inputPath string, opts Options) (Result, error) {
	text, err := readInput(inputPath)
	if err != nil {
		return Result{}, err
	}

	var blocks []Block
	if opts.FileHeader == "" {
		blocks, err = ParseAuto(text)
	} else {
		blocks, err = Parse(text, opts.FileHeader)
	}
	if err != nil {
		return Result{}, err
	}

	return Apply(blocks, opts)
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
	} else {
		b, err = os.ReadFile(path)
		if err != nil {
			return "", iof(err, "read input")
		}
	}
	return util.NormalizeNewlines(string(b)), nil
}
