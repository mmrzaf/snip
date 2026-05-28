package initwizard

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/bmatcuk/doublestar/v4"
)

var initSkipDirNames = map[string]bool{
	".git":          true,
	"node_modules":  true,
	"dist":          true,
	"build":         true,
	".venv":         true,
	"venv":          true,
	"__pycache__":   true,
	".pytest_cache": true,
	"coverage":      true,
	"target":        true,
	".snip":         true,
}

var initSkipFilePatterns = []string{
	"*.pyc",
	"*.pyo",
	"*.class",
	"*.o",
	"*.so",
	"*.dylib",
	"*.dll",
}

func collectRepoFiles(root string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || path == root {
			return nil
		}

		name := d.Name()
		if d.IsDir() {
			if initSkipDirNames[name] {
				return filepath.SkipDir
			}
			return nil
		}

		if !d.Type().IsRegular() {
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)

		for _, pat := range initSkipFilePatterns {
			ok, _ := doublestar.Match(pat, rel)
			if ok {
				return nil
			}
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
