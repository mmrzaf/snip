package discovery

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/go-git/go-billy/v5/osfs"
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"

	"github.com/mmrzaf/snip/internal/util"
)

// ExclusionReason describes why a file was excluded.
type ExclusionReason string

const (
	// ExcludedOutsideRoot indicates the file is not under root (should not happen with filepath.WalkDir).
	ExcludedOutsideRoot ExclusionReason = "excluded_outside_root"
	// ExcludedIgnoreAlways indicates the file matched ignore.always.
	ExcludedIgnoreAlways ExclusionReason = "excluded_ignore_always"
	// ExcludedSensitive indicates the file matched sensitive.exclude_globs.
	ExcludedSensitive ExclusionReason = "excluded_sensitive"
	// ExcludedGitignore indicates the file matched gitignore rules.
	ExcludedGitignore ExclusionReason = "excluded_gitignore"
	// ExcludedBinary indicates a file is binary by extension or sniffing.
	ExcludedBinary ExclusionReason = "excluded_binary"
	// ExcludedUnreadable indicates the file could not be opened/stat/read.
	ExcludedUnreadable ExclusionReason = "unreadable"
)

// PathInfo is a discovered file candidate.
type PathInfo struct {
	RelPath         string
	AbsPath         string
	SizeBytes       int64
	IsHidden        bool
	Excluded        bool
	ExclusionReason ExclusionReason
	ExclusionDetail string
}

// Engine discovers files under a root applying ignore rules.
type Engine struct {
	root             string
	useGitignore     bool
	ignoreAlways     []string
	sensitiveGlobs   []string
	binaryExts       map[string]bool
	gitignoreMatcher gitignore.Matcher
}

// NewEngine builds a discovery engine for the given root.
func NewEngine(root string, useGitignore bool, ignoreAlways, sensitiveGlobs, binaryExts []string) (*Engine, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("abs root: %w", err)
	}
	extMap := map[string]bool{}
	for _, e := range binaryExts {
		if e == "" {
			continue
		}
		if !strings.HasPrefix(e, ".") {
			e = "." + e
		}
		extMap[strings.ToLower(e)] = true
	}

	var matcher gitignore.Matcher
	if useGitignore {
		fs := osfs.New(abs)
		pats, err := gitignore.ReadPatterns(fs, nil)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("read gitignore: %w", err)
		}
		matcher = gitignore.NewMatcher(pats)
	}

	return &Engine{
		root:             abs,
		useGitignore:     useGitignore,
		ignoreAlways:     ignoreAlways,
		sensitiveGlobs:   sensitiveGlobs,
		binaryExts:       extMap,
		gitignoreMatcher: matcher,
	}, nil
}

// Discover walks the root and returns discovered file candidates.
func (e *Engine) Discover() ([]PathInfo, error) {
	var out []PathInfo

	err := filepath.WalkDir(e.root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			// IO error when traversing: propagate, because root traversal itself failed.
			return err
		}
		if path == e.root {
			return nil
		}

		// Avoid following symlinks: they can escape root and/or loop.
		if d.Type()&os.ModeSymlink != 0 {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		rel, err := filepath.Rel(e.root, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)

		// Skip directories early if they match ignore always.
		if d.IsDir() {
			if e.matchesAny(rel+"/", e.ignoreAlways) {
				return filepath.SkipDir
			}
			if e.useGitignore && e.gitignoreMatcher != nil {
				parts := strings.Split(rel, "/")
				if e.gitignoreMatcher.Match(append(parts, ""), true) {
					return filepath.SkipDir
				}
			}
			return nil
		}

		pi := PathInfo{
			RelPath:  rel,
			AbsPath:  path,
			IsHidden: isHiddenRel(rel),
		}

		st, statErr := os.Stat(path)
		if statErr != nil {
			pi.Excluded = true
			pi.ExclusionReason = ExcludedUnreadable
			pi.ExclusionDetail = statErr.Error()
			out = append(out, pi)
			return nil
		}
		pi.SizeBytes = st.Size()

		// Apply ignore order from ARCHITECTURE.md §8.2.
		if e.matchesAny(rel, e.ignoreAlways) {
			pi.Excluded = true
			pi.ExclusionReason = ExcludedIgnoreAlways
			pi.ExclusionDetail = "ignore.always"
			out = append(out, pi)
			return nil
		}
		if e.matchesAny(rel, e.sensitiveGlobs) {
			pi.Excluded = true
			pi.ExclusionReason = ExcludedSensitive
			pi.ExclusionDetail = "sensitive.exclude_globs"
			out = append(out, pi)
			return nil
		}
		if e.useGitignore && e.gitignoreMatcher != nil {
			parts := strings.Split(rel, "/")
			if e.gitignoreMatcher.Match(parts, false) {
				pi.Excluded = true
				pi.ExclusionReason = ExcludedGitignore
				pi.ExclusionDetail = ".gitignore"
				out = append(out, pi)
				return nil
			}
		}

		ext := strings.ToLower(filepath.Ext(rel))
		if e.binaryExts[ext] {
			pi.Excluded = true
			pi.ExclusionReason = ExcludedBinary
			pi.ExclusionDetail = "binary extension"
			out = append(out, pi)
			return nil
		}
		isBin, sniffErr := sniffBinary(path)
		if sniffErr != nil {
			pi.Excluded = true
			pi.ExclusionReason = ExcludedUnreadable
			pi.ExclusionDetail = sniffErr.Error()
			out = append(out, pi)
			return nil
		}
		if isBin {
			pi.Excluded = true
			pi.ExclusionReason = ExcludedBinary
			pi.ExclusionDetail = "binary sniff"
			out = append(out, pi)
			return nil
		}

		out = append(out, pi)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk: %w", err)
	}

	// Determinism: sort by relpath.
	// (The caller may further group/order included files.)
	sortPathInfos(out)
	return out, nil
}

func isHiddenRel(rel string) bool {
	rel = strings.TrimPrefix(rel, "./")
	for _, seg := range strings.Split(rel, "/") {
		if seg == "" || seg == "." || seg == ".." {
			continue
		}
		if strings.HasPrefix(seg, ".") {
			return true
		}
	}
	return false
}

func (e *Engine) matchesAny(rel string, patterns []string) bool {
	for _, pat := range patterns {
		if pat == "" {
			continue
		}
		ok, err := doublestar.Match(pat, rel)
		if err != nil {
			continue
		}
		if ok {
			return true
		}
	}
	return false
}

func sniffBinary(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer func() { _ = f.Close() }()

	const n = 8 * 1024
	buf := make([]byte, n)
	r, err := f.Read(buf)
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	buf = buf[:r]
	return util.SniffBinary(buf), nil
}

func sortPathInfos(in []PathInfo) {
	// Simple insertion sort to avoid pulling in sort just for one file; repo is small.
	for i := 1; i < len(in); i++ {
		j := i
		for j > 0 && in[j].RelPath < in[j-1].RelPath {
			in[j], in[j-1] = in[j-1], in[j]
			j--
		}
	}
}
