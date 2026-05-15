package initwizard

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/bmatcuk/doublestar/v4"
)

func collectRepoFiles(root string) ([]string, error) {
	ignorePatterns := []string{
		".git/**", "node_modules/**", "dist/**", "build/**", ".venv/**", "venv/**",
		"__pycache__/**", "*.pyc", ".pytest_cache/**", "coverage/**", "target/**", ".snip/**",
	}
	var out []string
	skipDir := func(rel string) bool {
		for _, pat := range ignorePatterns {
			ok, _ := doublestar.Match(pat, rel)
			if ok {
				return true
			}

		}
		return false
	}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || path == root {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if skipDir(rel) {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		out = append(out, rel)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(out)
	return out, nil
}
