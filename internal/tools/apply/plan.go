package apply

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/mmrzaf/snip/internal/util"
)

// PlannedFile is a validated filesystem operation.
type PlannedFile struct {
	RelPath   string
	AbsPath   string
	Content   []byte
	Exists    bool
	Overwrite bool
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
			return Result{}, invalidf("duplicate target path %q (blocks %d and %d)", rel, first+1, i+1)
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
