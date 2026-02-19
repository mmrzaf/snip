package initwizard

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/mmrzaf/snip/internal/config"
	"github.com/mmrzaf/snip/internal/selector"
)

// Options control init behavior.
type Options struct {
	Root           string
	Force          bool
	NonInteractive bool
	ProfileDefault string
}

// Run creates a .snip.yaml configuration in the root directory.
// It performs an evidence-based scan to pre-populate slices and profiles.
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

	choices := defaultChoices(absRoot)

	if !opts.NonInteractive {
		in := bufio.NewReader(os.Stdin)
		choices.IntentProfile = promptOneOf(in, "Primary intent / default profile [api/tests/full/minimal/debug]", choices.IntentProfile,
			[]string{"api", "tests", "full", "minimal", "debug"})

		choices.OutputDir = promptString(in, "Output dir", choices.OutputDir)
		choices.OutputPattern = promptString(in, "Output pattern", choices.OutputPattern)

		latest := promptYesNo(in, "Write latest alias file (last.md)", choices.WriteLatest)
		choices.WriteLatest = latest

		choices.TreeDepth = promptInt(in, "Tree depth", choices.TreeDepth, 1, 20)
		choices.MaxChars = promptBudget(in, "Max chars budget", choices.MaxChars)

		choices.DelimiterHeader = promptString(in, "File delimiter header (blank to disable)", choices.DelimiterHeader)
		choices.DelimiterFooter = promptString(in, "File delimiter footer (blank to disable)", choices.DelimiterFooter)
	}

	cfg, counts, err := generateConfig(absRoot, choices)
	if err != nil {
		return "", err
	}

	// Optional post-generation interactive slice toggle for the chosen default profile.
	if !opts.NonInteractive {
		in := bufio.NewReader(os.Stdin)
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Detected slices (file counts):")
		printSliceCounts(os.Stderr, cfg, counts)

		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintf(os.Stderr, "Default profile is %q; enabled slices: [%s]\n", cfg.DefaultProfile, strings.Join(cfg.Profiles[cfg.DefaultProfile].Enable, ", "))
		fmt.Fprintln(os.Stderr, "Adjust enabled slices for default profile using modifiers (e.g. +tests -docs). Blank to accept:")

		line, _ := in.ReadString('\n')
		line = strings.TrimSpace(line)
		if line != "" {
			fields := strings.Fields(line)
			mods, err := selector.ParseModifiers(fields)
			if err != nil {
				return "", fmt.Errorf("invalid modifiers: %w", err)
			}
			p := cfg.Profiles[cfg.DefaultProfile]
			p.Enable, err = applyModifiersToEnable(p.Enable, mods, cfg.Slices)
			if err != nil {
				return "", err
			}
			cfg.Profiles[cfg.DefaultProfile] = p

			// If user gave --profile-default flag, honor it last.
			if opts.ProfileDefault != "" {
				cfg.DefaultProfile = opts.ProfileDefault
			}
		}
	}

	if opts.ProfileDefault != "" {
		// Persist it.
		if _, ok := cfg.Profiles[opts.ProfileDefault]; !ok {
			return "", fmt.Errorf("unknown profile-default %q", opts.ProfileDefault)
		}
		cfg.DefaultProfile = opts.ProfileDefault
	}

	if err := config.Validate(cfg); err != nil {
		return "", fmt.Errorf("generated config invalid: %w", err)
	}
	if err := config.Write(outPath, cfg); err != nil {
		return "", err
	}
	return outPath, nil
}

type initChoices struct {
	IntentProfile   string
	OutputDir       string
	OutputPattern   string
	WriteLatest     bool
	TreeDepth       int
	MaxChars        int
	DelimiterHeader string
	DelimiterFooter string
}

func defaultChoices(absRoot string) initChoices {
	_ = absRoot
	return initChoices{
		IntentProfile:   "api",
		OutputDir:       ".snip",
		OutputPattern:   "snip_{profile}_{ts}_{gitsha}.md",
		WriteLatest:     true,
		TreeDepth:       4,
		MaxChars:        120000,
		DelimiterHeader: "<<<FILE:{path}>>>",
		DelimiterFooter: "",
	}
}

// generateConfig is evidence-based: it only emits include globs that match actual repo structure/signals.
// It returns cfg plus per-slice match counts (best-effort, used for interactive display).
func generateConfig(absRoot string, choices initChoices) (config.Config, map[string]int, error) {
	cfg := config.Default()
	cfg.Name = filepath.Base(absRoot)
	cfg.Root = "."
	cfg.DefaultProfile = choices.IntentProfile

	cfg.Output.Dir = choices.OutputDir
	cfg.Output.Pattern = choices.OutputPattern
	if choices.WriteLatest {
		cfg.Output.Latest = "last.md"
	} else {
		cfg.Output.Latest = ""
	}
	cfg.Render.TreeDepth = choices.TreeDepth
	cfg.Budgets.MaxChars = choices.MaxChars
	cfg.Render.FileBlock.Header = choices.DelimiterHeader
	cfg.Render.FileBlock.Footer = choices.DelimiterFooter

	paths, err := collectRepoFiles(absRoot, cfg.Ignore.Always)
	if err != nil {
		return config.Config{}, nil, err
	}

	slices := standardSlicesEmpty()

	// Evidence: directory existence.
	hasDir := func(rel string) bool {
		st, err := os.Stat(filepath.Join(absRoot, rel))
		return err == nil && st.IsDir()
	}
	hasFile := func(rel string) bool {
		st, err := os.Stat(filepath.Join(absRoot, rel))
		return err == nil && !st.IsDir()
	}

	isGoRepo := hasFile("go.mod")

	// api slice: include only existing layout dirs.
	if hasDir("cmd") {
		slices["api"] = addIncludes(slices["api"], []string{"cmd/**"})
	}
	if hasDir("internal") {
		slices["api"] = addIncludes(slices["api"], []string{"internal/**"})
	}
	if hasDir("pkg") {
		slices["api"] = addIncludes(slices["api"], []string{"pkg/**"})
	}
	// If none exist but src exists, use src as fallback.
	if len(slices["api"].Include) == 0 && hasDir("src") {
		slices["api"] = addIncludes(slices["api"], []string{"src/**"})
	}

	// cli slice: only if cmd exists.
	if hasDir("cmd") {
		slices["cli"] = addIncludes(slices["cli"], []string{"cmd/**"})
	}

	// docs: README* always ok; docs/** only if docs exists.
	slices["docs"] = addIncludes(slices["docs"], []string{"README*"})
	if hasDir("docs") {
		slices["docs"] = addIncludes(slices["docs"], []string{"docs/**"})
	}

	// tests: only add patterns if signals exist.
	if isGoRepo {
		// This is safe even if there are no tests.
		slices["tests"] = addIncludes(slices["tests"], []string{"**/*_test.go"})
	}
	if hasDir("tests") {
		slices["tests"] = addIncludes(slices["tests"], []string{"tests/**"})
	}

	// configs: only enable if any config-like files are present.
	if anyMatch(paths, []string{"**/*.yaml", "**/*.yml", "**/*.json", "**/*.toml", "**/*.ini"}) {
		slices["configs"] = addIncludes(slices["configs"], []string{"**/*.yaml", "**/*.yml", "**/*.json", "**/*.toml", "**/*.ini"})
	}

	// infra: only existing.
	for _, d := range []string{".github", ".gitlab", "deploy", "infra", "terraform"} {
		if hasDir(d) {
			slices["infra"] = addIncludes(slices["infra"], []string{d + "/**"})
		}
	}

	// scripts.
	if hasDir("scripts") {
		slices["scripts"] = addIncludes(slices["scripts"], []string{"scripts/**"})
	}
	if anyMatch(paths, []string{"**/*.sh"}) {
		slices["scripts"] = addIncludes(slices["scripts"], []string{"**/*.sh"})
	}

	// schema: only if signals exist.
	if anyMatch(paths, []string{"openapi.*", "swagger.*"}) {
		slices["schema"] = addIncludes(slices["schema"], []string{"openapi.*", "swagger.*"})
	}
	if hasDir("proto") || anyMatch(paths, []string{"**/*.proto"}) {
		slices["schema"] = addIncludes(slices["schema"], []string{"proto/**", "**/*.proto"})
	}
	if hasFile("schema.graphql") || anyMatch(paths, []string{"**/*.graphql"}) {
		slices["schema"] = addIncludes(slices["schema"], []string{"schema.graphql", "**/*.graphql"})
	}

	// domain/persistence: only when directories exist (no guessing).
	for _, d := range []string{"internal/domain", "src/domain"} {
		if hasDir(d) {
			slices["domain"] = addIncludes(slices["domain"], []string{d + "/**"})
		}
	}
	for _, d := range []string{"internal/persistence", "src/persistence", "migrations"} {
		if hasDir(d) {
			slices["persistence"] = addIncludes(slices["persistence"], []string{d + "/**"})
		}
	}

	cfg.Slices = slices

	// Count matches per slice for UI.
	counts := map[string]int{}
	for name, sl := range cfg.Slices {
		counts[name] = countMatches(paths, sl.Include)
	}

	cfg.Profiles = generateProfiles(cfg.Slices, counts, choices.IntentProfile)

	// Force-persist default profile exists.
	if _, ok := cfg.Profiles[cfg.DefaultProfile]; !ok {
		cfg.DefaultProfile = "api"
		if _, ok := cfg.Profiles[cfg.DefaultProfile]; !ok {
			// pick deterministic first
			cfg.DefaultProfile = firstProfile(cfg.Profiles)
		}
	}

	return cfg, counts, nil
}

func standardSlicesEmpty() map[string]config.SliceConfig {
	return map[string]config.SliceConfig{
		"api":         {Include: []string{}, Exclude: []string{}, Priority: 100},
		"tests":       {Include: []string{}, Exclude: []string{}, Priority: 40},
		"docs":        {Include: []string{}, Exclude: []string{}, Priority: 20},
		"schema":      {Include: []string{}, Exclude: []string{}, Priority: 25},
		"infra":       {Include: []string{}, Exclude: []string{}, Priority: 10},
		"configs":     {Include: []string{}, Exclude: []string{}, Priority: 15},
		"scripts":     {Include: []string{}, Exclude: []string{}, Priority: 5},
		"cli":         {Include: []string{}, Exclude: []string{}, Priority: 12},
		"domain":      {Include: []string{}, Exclude: []string{}, Priority: 18},
		"persistence": {Include: []string{}, Exclude: []string{}, Priority: 14},
	}
}

func generateProfiles(slices map[string]config.SliceConfig, counts map[string]int, intent string) map[string]config.Profile {
	nonEmpty := func(name string) bool {
		sl, ok := slices[name]
		if !ok {
			return false
		}
		// Consider "non-empty" if it has includes and matches at least one file.
		if len(sl.Include) == 0 {
			return false
		}
		return counts[name] > 0
	}

	// Helper: always include api slice if present; even if empty, so profiles remain usable.
	apiEnable := []string{"api"}
	if _, ok := slices["api"]; !ok {
		apiEnable = []string{}
	}

	// Baselines, but only enable relevant slices (non-empty).
	apiProfile := []string{}
	apiProfile = append(apiProfile, apiEnable...)
	for _, s := range []string{"docs", "configs", "schema"} {
		if nonEmpty(s) {
			apiProfile = append(apiProfile, s)
		}
	}
	if len(apiProfile) == 0 {
		// fallback: enable any slice that exists.
		apiProfile = append(apiProfile, firstSliceName(slices))
	}

	testsProfile := []string{}
	if nonEmpty("tests") {
		testsProfile = append(testsProfile, "tests")
	}
	testsProfile = appendUnique(testsProfile, apiEnable...)
	if nonEmpty("domain") {
		testsProfile = appendUnique(testsProfile, "domain")
	}
	if len(testsProfile) == 0 {
		testsProfile = append(testsProfile, apiProfile...)
	}

	fullProfile := []string{}
	// Enable all non-empty slices; if none, enable api.
	for name := range slices {
		if nonEmpty(name) {
			fullProfile = append(fullProfile, name)
		}
	}
	sort.Strings(fullProfile)
	if len(fullProfile) == 0 {
		fullProfile = append(fullProfile, apiProfile...)
	}

	minimalProfile := []string{}
	minimalProfile = append(minimalProfile, apiEnable...)
	if len(minimalProfile) == 0 {
		minimalProfile = append(minimalProfile, apiProfile...)
	}

	debugProfile := []string{}
	debugProfile = append(debugProfile, apiEnable...)
	for _, s := range []string{"tests", "docs", "schema"} {
		if nonEmpty(s) {
			debugProfile = appendUnique(debugProfile, s)
		}
	}
	if len(debugProfile) == 0 {
		debugProfile = append(debugProfile, fullProfile...)
	}

	// Budgets per architecture defaults.
	profiles := map[string]config.Profile{
		"api": {
			Enable: apiProfile,
			Budgets: config.BudgetOverride{
				MaxChars: 120000,
			},
			Render: config.RenderOverride{TreeDepth: 4},
		},
		"tests": {
			Enable: testsProfile,
			Budgets: config.BudgetOverride{
				MaxChars: 120000,
			},
			Render: config.RenderOverride{TreeDepth: 4},
		},
		"full": {
			Enable: fullProfile,
			Budgets: config.BudgetOverride{
				MaxChars: 220000,
			},
			Render: config.RenderOverride{TreeDepth: 5},
		},
		"minimal": {
			Enable: minimalProfile,
			Budgets: config.BudgetOverride{
				MaxChars: 80000,
			},
			Render: config.RenderOverride{TreeDepth: 3},
		},
		"debug": {
			Enable: debugProfile,
			Budgets: config.BudgetOverride{
				MaxChars: 220000,
			},
			Render: config.RenderOverride{TreeDepth: 5},
		},
	}

	return profiles
}

func appendUnique(in []string, values ...string) []string {
	for _, v := range values {
		found := false
		for _, x := range in {
			if x == v {
				found = true
				break
			}
		}
		if !found {
			in = append(in, v)
		}
	}
	return in
}

func firstProfile(m map[string]config.Profile) string {
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

func firstSliceName(m map[string]config.SliceConfig) string {
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

func addIncludes(sl config.SliceConfig, pats []string) config.SliceConfig {
	seen := map[string]bool{}
	for _, p := range sl.Include {
		seen[p] = true
	}
	for _, p := range pats {
		if p == "" {
			continue
		}
		if !seen[p] {
			sl.Include = append(sl.Include, p)
			seen[p] = true
		}
	}
	sort.Strings(sl.Include)
	return sl
}

func collectRepoFiles(absRoot string, ignoreAlways []string) ([]string, error) {
	var out []string
	skipDir := func(rel string) bool {
		rel = strings.TrimSuffix(rel, "/")
		if rel == "" {
			return false
		}
		relSlash := filepath.ToSlash(rel)
		for _, pat := range ignoreAlways {
			ok, err := doublestar.Match(pat, relSlash+"/")
			if err != nil {
				continue
			}
			if ok {
				return true
			}
		}
		return false
	}

	err := filepath.WalkDir(absRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			// Init should be resilient: ignore traversal errors.
			return nil
		}
		if path == absRoot {
			return nil
		}
		rel, rerr := filepath.Rel(absRoot, path)
		if rerr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)

		if d.Type()&os.ModeSymlink != 0 {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if skipDir(rel) {
				return filepath.SkipDir
			}
			return nil
		}
		// Only keep regular files.
		info, ierr := d.Info()
		if ierr != nil {
			return nil
		}
		if !info.Mode().IsRegular() {
			return nil
		}

		out = append(out, rel)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk: %w", err)
	}

	sort.Strings(out)
	return out, nil
}

func anyMatch(paths []string, patterns []string) bool {
	return countMatches(paths, patterns) > 0
}

func countMatches(paths []string, patterns []string) int {
	if len(patterns) == 0 {
		return 0
	}
	var c int
	for _, p := range paths {
		for _, pat := range patterns {
			ok, err := doublestar.Match(pat, p)
			if err != nil {
				continue
			}
			if ok {
				c++
				break
			}
		}
	}
	return c
}

func printSliceCounts(w *os.File, cfg config.Config, counts map[string]int) {
	type row struct {
		name     string
		enabled  bool
		priority int
		count    int
		includes []string
	}
	var rows []row
	for name, sl := range cfg.Slices {
		rows = append(rows, row{
			name:     name,
			enabled:  contains(cfg.Profiles[cfg.DefaultProfile].Enable, name),
			priority: sl.Priority,
			count:    counts[name],
			includes: sl.Include,
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].priority != rows[j].priority {
			return rows[i].priority > rows[j].priority
		}
		return rows[i].name < rows[j].name
	})
	for _, r := range rows {
		mark := " "
		if r.enabled {
			mark = "x"
		}
		_, _ = fmt.Fprintf(w, "  [%s] %-12s  files=%-5d  include=%v\n", mark, r.name, r.count, r.includes)
	}
}

func contains(in []string, v string) bool {
	for _, x := range in {
		if x == v {
			return true
		}
	}
	return false
}

func applyModifiersToEnable(current []string, mods []selector.Modifier, slices map[string]config.SliceConfig) ([]string, error) {
	enabled := map[string]bool{}
	for _, s := range current {
		enabled[s] = true
	}
	for _, m := range mods {
		if _, ok := slices[m.Name]; !ok {
			return nil, fmt.Errorf("unknown slice %q", m.Name)
		}
		enabled[m.Name] = m.Enable
	}
	var out []string
	for s, on := range enabled {
		if on {
			out = append(out, s)
		}
	}
	sort.Strings(out)
	if len(out) == 0 {
		return nil, fmt.Errorf("default profile must enable at least one slice")
	}
	return out, nil
}

// ---- prompts ----

func promptString(in *bufio.Reader, label string, def string) string {
	fmt.Fprintf(os.Stderr, "%s [%s]: ", label, def)
	line, err := in.ReadString('\n')
	if err != nil {
		return def
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return def
	}
	return line
}

func promptOneOf(in *bufio.Reader, label string, def string, allowed []string) string {
	allowedSet := map[string]bool{}
	for _, a := range allowed {
		allowedSet[a] = true
	}
	for {
		fmt.Fprintf(os.Stderr, "%s [%s]: ", label, def)
		line, err := in.ReadString('\n')
		if err != nil {
			return def
		}
		line = strings.TrimSpace(line)
		if line == "" {
			return def
		}
		if allowedSet[line] {
			return line
		}
		fmt.Fprintf(os.Stderr, "Invalid value %q. Allowed: %s\n", line, strings.Join(allowed, ", "))
	}
}

func promptYesNo(in *bufio.Reader, label string, def bool) bool {
	defStr := "y"
	if !def {
		defStr = "n"
	}
	for {
		fmt.Fprintf(os.Stderr, "%s [y/n] [%s]: ", label, defStr)
		line, err := in.ReadString('\n')
		if err != nil {
			return def
		}
		line = strings.TrimSpace(strings.ToLower(line))
		if line == "" {
			return def
		}
		if line == "y" || line == "yes" {
			return true
		}
		if line == "n" || line == "no" {
			return false
		}
		fmt.Fprintln(os.Stderr, "Enter y or n.")
	}
}

func promptInt(in *bufio.Reader, label string, def int, min int, max int) int {
	for {
		fmt.Fprintf(os.Stderr, "%s [%d]: ", label, def)
		line, err := in.ReadString('\n')
		if err != nil {
			return def
		}
		line = strings.TrimSpace(line)
		if line == "" {
			return def
		}
		v, err := strconv.Atoi(line)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Enter a number.")
			continue
		}
		if v < min || v > max {
			fmt.Fprintf(os.Stderr, "Enter a number between %d and %d.\n", min, max)
			continue
		}
		return v
	}
}

func promptBudget(in *bufio.Reader, label string, def int) int {
	for {
		fmt.Fprintf(os.Stderr, "%s (e.g. 120000, 200000) [%d]: ", label, def)
		line, err := in.ReadString('\n')
		if err != nil {
			return def
		}
		line = strings.TrimSpace(line)
		if line == "" {
			return def
		}
		v, err := strconv.Atoi(line)
		if err != nil || v <= 0 {
			fmt.Fprintln(os.Stderr, "Enter a positive integer.")
			continue
		}
		return v
	}
}
