package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config is the root configuration schema for snip.
//
// It matches ARCHITECTURE.md §6 and is intended to be stable for v1.
// Additions are backward-compatible (optional fields).
type Config struct {
	Version        int                    `yaml:"version"`
	Root           string                 `yaml:"root"`
	Name           string                 `yaml:"name"`
	DefaultProfile string                 `yaml:"default_profile"`
	Output         OutputConfig           `yaml:"output"`
	Render         RenderConfig           `yaml:"render"`
	Budgets        BudgetConfig           `yaml:"budgets"`
	Ignore         IgnoreConfig           `yaml:"ignore"`
	Sensitive      SensitiveConfig        `yaml:"sensitive"`
	Slices         map[string]SliceConfig `yaml:"slices"`
	Profiles       map[string]Profile     `yaml:"profiles"`
}

// OutputConfig controls where bundles are written.
type OutputConfig struct {
	Dir           string `yaml:"dir"`
	Pattern       string `yaml:"pattern"`
	Latest        string `yaml:"latest"`
	StdoutDefault bool   `yaml:"stdout_default"`
}

// RenderConfig controls markdown rendering.
type RenderConfig struct {
	Format          string          `yaml:"format"`
	Newline         string          `yaml:"newline"`
	CodeFences      bool            `yaml:"code_fences"`
	IncludeTree     bool            `yaml:"include_tree"`
	TreeDepth       int             `yaml:"tree_depth"`
	IncludeManifest bool            `yaml:"include_manifest"`
	Manifest        ManifestConfig  `yaml:"manifest"`
	FileBlock       FileBlockConfig `yaml:"file_block"`
}

// FileBlockConfig customizes per-file delimiter markers.
type FileBlockConfig struct {
	Header string `yaml:"header"`
	Footer string `yaml:"footer"`
}

// ManifestConfig controls manifest rendering.
type ManifestConfig struct {
	GroupBySlice           bool `yaml:"group_by_slice"`
	IncludeLineCounts      bool `yaml:"include_line_counts"`
	IncludeByteCounts      bool `yaml:"include_byte_counts"`
	IncludeTruncationNotes bool `yaml:"include_truncation_notes"`
	IncludeUnreadableNotes bool `yaml:"include_unreadable_notes"`
}

// BudgetConfig controls output budgets.
type BudgetConfig struct {
	MaxChars        int    `yaml:"max_chars"`
	PerFileMaxLines int    `yaml:"per_file_max_lines"`
	PerFileMaxBytes int    `yaml:"per_file_max_bytes"`
	DropPolicy      string `yaml:"drop_policy"`
}

// IgnoreConfig controls ignore rules.
type IgnoreConfig struct {
	UseGitignore     bool     `yaml:"use_gitignore"`
	Always           []string `yaml:"always"`
	BinaryExtensions []string `yaml:"binary_extensions"`
}

// SensitiveConfig controls sensitive exclusions.
type SensitiveConfig struct {
	ExcludeGlobs []string `yaml:"exclude_globs"`
}

// SliceConfig defines a slice.
type SliceConfig struct {
	Include  []string `yaml:"include"`
	Exclude  []string `yaml:"exclude"`
	Priority int      `yaml:"priority"`
}

// Profile defines a profile.
type Profile struct {
	Enable  []string       `yaml:"enable"`
	Budgets BudgetOverride `yaml:"budgets"`
	Render  RenderOverride `yaml:"render"`
}

// BudgetOverride allows per-profile overrides.
type BudgetOverride struct {
	MaxChars int `yaml:"max_chars"`
}

// RenderOverride allows per-profile overrides.
type RenderOverride struct {
	TreeDepth int `yaml:"tree_depth"`
}

// Default returns a conservative default config.
func Default() Config {
	return Config{
		Version:        1,
		Root:           ".",
		Name:           "",
		DefaultProfile: "api",
		Output: OutputConfig{
			Dir:           ".snip",
			Pattern:       "snip_{profile}_{ts}_{gitsha}.md",
			Latest:        "last.md",
			StdoutDefault: false,
		},
		Render: RenderConfig{
			Format:          "md",
			Newline:         "\n",
			CodeFences:      true,
			IncludeTree:     true,
			TreeDepth:       4,
			IncludeManifest: true,
			Manifest: ManifestConfig{
				GroupBySlice:           true,
				IncludeLineCounts:      true,
				IncludeByteCounts:      true,
				IncludeTruncationNotes: true,
				IncludeUnreadableNotes: true,
			},
			FileBlock: FileBlockConfig{},
		},
		Budgets: BudgetConfig{
			MaxChars:        120000,
			PerFileMaxLines: 600,
			PerFileMaxBytes: 262144,
			DropPolicy:      "drop_low_priority",
		},
		Ignore: IgnoreConfig{
			UseGitignore: true,
			Always: []string{
				".git/**",
				"node_modules/**",
				"dist/**",
				"build/**",
				".venv/**",
				".pytest_cache/**",
				"coverage/**",
				"target/**",
				".snip/**",
			},
			BinaryExtensions: []string{
				".png", ".jpg", ".jpeg", ".gif", ".pdf", ".zip", ".tar", ".gz", ".7z",
				".exe", ".dll", ".so", ".dylib",
			},
		},
		Sensitive: SensitiveConfig{
			ExcludeGlobs: []string{
				".env*",
				"**/*secret*",
				"**/*secrets*",
				"**/*.pem",
				"**/*.key",
				"**/id_rsa*",
				"**/*serviceAccount*.json",
			},
		},
		Slices:   map[string]SliceConfig{},
		Profiles: map[string]Profile{},
	}
}

// Load reads and validates a config file.
func Load(path string) (Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse yaml: %w", err)
	}
	if cfg.Version == 0 {
		cfg.Version = 1
	}
	if cfg.Version != 1 {
		return Config{}, fmt.Errorf("unsupported config version %d", cfg.Version)
	}
	cfg = mergeDefaults(cfg)
	if err := Validate(cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func mergeDefaults(cfg Config) Config {
	def := Default()

	if cfg.Root == "" {
		cfg.Root = def.Root
	}
	if cfg.Output.Dir == "" {
		cfg.Output.Dir = def.Output.Dir
	}
	if cfg.Output.Pattern == "" {
		cfg.Output.Pattern = def.Output.Pattern
	}
	if cfg.Render.Format == "" {
		cfg.Render.Format = def.Render.Format
	}
	if cfg.Render.Newline == "" {
		cfg.Render.Newline = def.Render.Newline
	}
	if cfg.Budgets.MaxChars == 0 {
		cfg.Budgets.MaxChars = def.Budgets.MaxChars
	}
	if cfg.Budgets.PerFileMaxLines == 0 {
		cfg.Budgets.PerFileMaxLines = def.Budgets.PerFileMaxLines
	}
	if cfg.Budgets.PerFileMaxBytes == 0 {
		cfg.Budgets.PerFileMaxBytes = def.Budgets.PerFileMaxBytes
	}
	if cfg.Budgets.DropPolicy == "" {
		cfg.Budgets.DropPolicy = def.Budgets.DropPolicy
	}
	if cfg.Ignore.Always == nil {
		cfg.Ignore.Always = def.Ignore.Always
	}
	if cfg.Ignore.BinaryExtensions == nil {
		cfg.Ignore.BinaryExtensions = def.Ignore.BinaryExtensions
	}
	if cfg.Sensitive.ExcludeGlobs == nil {
		cfg.Sensitive.ExcludeGlobs = def.Sensitive.ExcludeGlobs
	}
	if cfg.Slices == nil {
		cfg.Slices = map[string]SliceConfig{}
	}
	if cfg.Profiles == nil {
		cfg.Profiles = map[string]Profile{}
	}
	if cfg.DefaultProfile == "" {
		// Backward-compatible defaulting.
		if _, ok := cfg.Profiles["api"]; ok {
			cfg.DefaultProfile = "api"
		} else if len(cfg.Profiles) > 0 {
			cfg.DefaultProfile = firstProfileName(cfg.Profiles)
		}
	}

	return cfg
}

func firstProfileName(m map[string]Profile) string {
	names := make([]string, 0, len(m))
	for k := range m {
		names = append(names, k)
	}
	sort.Strings(names)
	if len(names) == 0 {
		return ""
	}
	return names[0]
}

// Validate enforces basic schema constraints.
func Validate(cfg Config) error {
	if cfg.Version != 1 {
		return fmt.Errorf("version must be 1")
	}
	if cfg.Budgets.MaxChars <= 0 {
		return fmt.Errorf("budgets.max_chars must be > 0")
	}
	if cfg.Budgets.PerFileMaxLines <= 0 {
		return fmt.Errorf("budgets.per_file_max_lines must be > 0")
	}
	if cfg.Budgets.PerFileMaxBytes <= 0 {
		return fmt.Errorf("budgets.per_file_max_bytes must be > 0")
	}
	if cfg.Render.Format != "md" {
		return fmt.Errorf("render.format must be 'md'")
	}
	if cfg.Output.Pattern == "" {
		return fmt.Errorf("output.pattern is required")
	}

	// Validate delimiter strings: must be single-line to keep output parseable.
	if strings.ContainsAny(cfg.Render.FileBlock.Header, "\r\n") {
		return fmt.Errorf("render.file_block.header must not contain newlines")
	}
	if strings.ContainsAny(cfg.Render.FileBlock.Footer, "\r\n") {
		return fmt.Errorf("render.file_block.footer must not contain newlines")
	}

	if len(cfg.Slices) == 0 {
		return fmt.Errorf("at least one slice is required")
	}
	if len(cfg.Profiles) == 0 {
		return fmt.Errorf("at least one profile is required")
	}

	for name := range cfg.Slices {
		if name == "" {
			return fmt.Errorf("slice name cannot be empty")
		}
		// NOTE: allow empty include list (init creates standard slices but leaves absent ones empty).
	}

	for name, p := range cfg.Profiles {
		if name == "" {
			return fmt.Errorf("profile name cannot be empty")
		}
		if len(p.Enable) == 0 {
			return fmt.Errorf("profile %q must enable at least one slice", name)
		}
		for _, s := range p.Enable {
			if _, ok := cfg.Slices[s]; !ok {
				return fmt.Errorf("profile %q enables unknown slice %q", name, s)
			}
		}
	}

	if cfg.DefaultProfile != "" {
		if _, ok := cfg.Profiles[cfg.DefaultProfile]; !ok {
			return fmt.Errorf("default_profile %q does not exist", cfg.DefaultProfile)
		}
	}

	// Policy name check (v1 only supports drop_low_priority).
	if cfg.Budgets.DropPolicy != "drop_low_priority" {
		return fmt.Errorf("budgets.drop_policy must be 'drop_low_priority'")
	}

	return nil
}

// EffectiveRoot resolves the effective root directory.
func EffectiveRoot(cfg Config, override string) (string, error) {
	r := cfg.Root
	if override != "" {
		r = override
	}
	if r == "" {
		r = "."
	}
	abs, err := filepath.Abs(r)
	if err != nil {
		return "", fmt.Errorf("abs root: %w", err)
	}
	st, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("stat root: %w", err)
	}
	if !st.IsDir() {
		return "", fmt.Errorf("root is not a directory: %s", abs)
	}
	return abs, nil
}

// ApplyProfileOverrides applies profile overrides to config and returns a new config.
func ApplyProfileOverrides(cfg Config, profile string) (Config, error) {
	p, ok := cfg.Profiles[profile]
	if !ok {
		return Config{}, fmt.Errorf("unknown profile %q", profile)
	}
	out := cfg
	if p.Budgets.MaxChars > 0 {
		out.Budgets.MaxChars = p.Budgets.MaxChars
	}
	if p.Render.TreeDepth > 0 {
		out.Render.TreeDepth = p.Render.TreeDepth
	}
	return out, nil
}

// Write writes the config to disk with safe permissions.
func Write(path string, cfg Config) error {
	b, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal yaml: %w", err)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return fmt.Errorf("write temp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}

// EnsureNoSymlinkRoot rejects symlink roots for safety.
func EnsureNoSymlinkRoot(path string) error {
	fi, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return errors.New("root must not be a symlink")
	}
	return nil
}
