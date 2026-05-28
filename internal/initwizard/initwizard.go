package initwizard

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mmrzaf/snip/internal/config"
)

var (
	// ErrConfigExists is returned when a config file already exists and force is not enabled.
	ErrConfigExists = errors.New("config already exists")
	// ErrInvalidProfileDefault is returned when the requested default profile is not available.
	ErrInvalidProfileDefault = errors.New("invalid profile-default")
	// ErrInvalidInteractiveInput is returned when interactive input cannot be parsed.
	ErrInvalidInteractiveInput = errors.New("invalid interactive input")
)

// Options control init behavior.
type Options struct {
	Root           string
	Force          bool
	Interactive    bool
	NonInteractive bool // Kept for compatibility. Init is non-interactive by default.
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

	if st, err := os.Stat(absRoot); err != nil {
		return "", fmt.Errorf("stat root: %w", err)
	} else if !st.IsDir() {
		return "", fmt.Errorf("root is not a directory: %s", absRoot)
	}

	outPath := filepath.Join(absRoot, ".snip.yaml")
	if !opts.Force {
		if _, err := os.Stat(outPath); err == nil {
			return "", fmt.Errorf("%w: %s (use --force)", ErrConfigExists, outPath)
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("stat config: %w", err)
		}
	}

	paths, err := collectRepoFiles(absRoot)
	if err != nil {
		return "", err
	}

	project := detectProject(paths, opts.ProjectType)
	slices := buildSlices(project, paths)
	profiles := buildProfiles(project, slices)

	cfg := config.Default()
	cfg.Name = filepath.Base(absRoot)
	cfg.Root = "."
	cfg.Slices = slices
	cfg.Profiles = profiles
	cfg.DefaultProfile = "api"

	if opts.ProfileDefault != "" {
		if _, ok := cfg.Profiles[opts.ProfileDefault]; !ok {
			return "", fmt.Errorf("%w %q", ErrInvalidProfileDefault, opts.ProfileDefault)
		}
		cfg.DefaultProfile = opts.ProfileDefault
	}

	if opts.Interactive {
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
