package initwizard

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/mmrzaf/snip/internal/config"
)

// ProjectSignals describes high-level project characteristics.
// Used only by templates.go for future extensions.
type ProjectSignals struct {
	Go       bool
	Node     bool
	Python   bool
	Docker   bool
	Github   bool
	Monorepo bool
}

// BuildPlan is a structured representation of slices and profiles.
// Used only by templates.go for future extensions.
type BuildPlan struct {
	DefaultProfile string
	Slices         map[string][]string
	Profiles       map[string][]string
}

// Options control init behavior.
type Options struct {
	Root           string
	Force          bool
	NonInteractive bool
	ProfileDefault string
	ProjectType    string // optional hint: "go", "python", "node", etc.
}

// Run creates a .snip.yaml configuration in the root directory.
func Run(opts Options) (string, error) {
	root := opts.Root
	if root == "" {
		root = "."
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("abs root: %w", err)
	}

	outPath := filepath.Join(absRoot, ".snip.yaml")
	if !opts.Force {
		if _, err := os.Stat(outPath); err == nil {
			return "", fmt.Errorf("config already exists: %s (use --force)", outPath)
		}
	}

	project := detectProject(absRoot, opts.ProjectType)

	paths, err := collectRepoFiles(absRoot)
	if err != nil {
		return "", err
	}

	slices := buildSlices(project, paths)

	profiles := buildProfiles(project, slices)

	cfg := config.Default()
	cfg.Name = filepath.Base(absRoot)
	cfg.Root = "."
	cfg.Slices = slices
	cfg.Profiles = profiles
	cfg.DefaultProfile = "default"

	if opts.ProfileDefault != "" {
		if _, ok := cfg.Profiles[opts.ProfileDefault]; !ok {
			return "", fmt.Errorf("unknown profile-default %q", opts.ProfileDefault)
		}
		cfg.DefaultProfile = opts.ProfileDefault
	}

	if !opts.NonInteractive {
		if err := interactiveReview(&cfg, slices, profiles); err != nil {
			return "", err
		}
	}

	if err := config.Validate(cfg); err != nil {
		return "", fmt.Errorf("generated config invalid: %w", err)
	}
	if err := config.Write(outPath, cfg); err != nil {
		return "", err
	}
	return outPath, nil
}
