package initwizard

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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

	// Detect project type (or use explicit hint)
	project := detectProject(absRoot, opts.ProjectType)

	// Gather all files (respecting a sensible default ignore list)
	paths, err := collectRepoFiles(absRoot)
	if err != nil {
		return "", err
	}

	// Build slices tailored to the project
	slices := buildSlices(project, paths)

	// Build profiles from available slices
	profiles := buildProfiles(project, slices)

	// Assemble the configuration
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

	if err := config.Validate(cfg); err != nil {
		return "", fmt.Errorf("generated config invalid: %w", err)
	}
	if err := config.Write(outPath, cfg); err != nil {
		return "", err
	}
	return outPath, nil
}

// ----------------------------------------------------------------------
// Project detection
// ----------------------------------------------------------------------

type ProjectKind string

const (
	KindUnknown ProjectKind = "unknown"
	KindGo      ProjectKind = "go"
	KindPython  ProjectKind = "python"
	KindNode    ProjectKind = "node"
	KindRust    ProjectKind = "rust"
	KindJava    ProjectKind = "java"
	KindRuby    ProjectKind = "ruby"
	KindPHP     ProjectKind = "php"
	KindDotNet  ProjectKind = "dotnet"
)

type ProjectInfo struct {
	Kind       ProjectKind
	IsService  bool // has a main package, server entrypoint, etc.
	IsLibrary  bool // no main, likely a library
	IsWebApp   bool // frontend framework indicators
	HasTests   bool // evidence of tests
	HasDocs    bool // evidence of docs
	HasConfigs bool // config files present
	HasInfra   bool // CI/CD or container files
}

func detectProject(root string, hint string) ProjectInfo {
	info := ProjectInfo{Kind: KindUnknown}

	// If hint provided, use it.
	if hint != "" {
		switch strings.ToLower(hint) {
		case "go", "golang":
			info.Kind = KindGo
		case "python", "py":
			info.Kind = KindPython
		case "node", "nodejs", "javascript", "typescript":
			info.Kind = KindNode
		case "rust", "rs":
			info.Kind = KindRust
		case "java":
			info.Kind = KindJava
		case "ruby", "rb":
			info.Kind = KindRuby
		case "php":
			info.Kind = KindPHP
		case "dotnet", "csharp", "cs":
			info.Kind = KindDotNet
		}
	} else {
		info.Kind = detectLanguage(root)
	}

	// Further characteristics
	info.IsService = hasMainIndicator(root, info.Kind)
	info.IsLibrary = !info.IsService && hasLibraryIndicator(root, info.Kind)
	info.IsWebApp = isWebApp(root, info.Kind)
	info.HasTests = hasTestsIndicator(root, info.Kind)
	info.HasDocs = hasDocsIndicator(root)
	info.HasConfigs = hasConfigsIndicator(root)
	info.HasInfra = hasInfraIndicator(root)

	return info
}

func detectLanguage(root string) ProjectKind {
	markers := map[string]ProjectKind{
		"go.mod":           KindGo,
		"requirements.txt": KindPython,
		"setup.py":         KindPython,
		"pyproject.toml":   KindPython,
		"Pipfile":          KindPython,
		"package.json":     KindNode,
		"yarn.lock":        KindNode,
		"pnpm-lock.yaml":   KindNode,
		"Cargo.toml":       KindRust,
		"pom.xml":          KindJava,
		"build.gradle":     KindJava,
		"Gemfile":          KindRuby,
		"composer.json":    KindPHP,
	}
	for file, kind := range markers {
		if _, err := os.Stat(filepath.Join(root, file)); err == nil {
			return kind
		}
	}
	// Check for .csproj or .sln
	if matches, _ := filepath.Glob(filepath.Join(root, "*.csproj")); len(matches) > 0 {
		return KindDotNet
	}
	if matches, _ := filepath.Glob(filepath.Join(root, "*.sln")); len(matches) > 0 {
		return KindDotNet
	}
	return KindUnknown
}

func hasMainIndicator(root string, kind ProjectKind) bool {
	switch kind {
	case KindGo:
		return dirExists(root, "cmd") || fileExists(root, "main.go")
	case KindPython:
		return fileExists(root, "app.py") || fileExists(root, "main.py") ||
			dirExists(root, "routers") || dirExists(root, "api")
	case KindNode:
		return fileExists(root, "server.js") || fileExists(root, "index.js") ||
			dirExists(root, "pages/api") || dirExists(root, "app/api")
	case KindRust:
		return fileExists(root, "src/main.rs")
	case KindJava:
		return dirExists(root, "src/main/java")
	case KindRuby:
		return fileExists(root, "config.ru") || dirExists(root, "app/controllers")
	case KindPHP:
		return dirExists(root, "public") || fileExists(root, "index.php")
	case KindDotNet:
		return fileExists(root, "Program.cs") || fileExists(root, "Startup.cs")
	default:
		return false
	}
}

func hasLibraryIndicator(root string, kind ProjectKind) bool {
	// Opposite of service indicators, plus common lib patterns
	switch kind {
	case KindGo:
		return !hasMainIndicator(root, kind) && (dirExists(root, "pkg") || dirExists(root, "internal"))
	case KindPython:
		return dirExists(root, "src") && !hasMainIndicator(root, kind)
	case KindNode:
		return fileExists(root, "index.js") && !hasMainIndicator(root, kind)
	default:
		return false
	}
}

func isWebApp(root string, kind ProjectKind) bool {
	if kind != KindNode {
		return false
	}
	markers := []string{
		"vite.config.*", "webpack.config.js", "next.config.js",
		"src/App.jsx", "src/App.tsx", "src/App.vue", "angular.json",
	}
	for _, pat := range markers {
		if matches, _ := filepath.Glob(filepath.Join(root, pat)); len(matches) > 0 {
			return true
		}
	}
	return false
}

func hasTestsIndicator(root string, kind ProjectKind) bool {
	switch kind {
	case KindGo:
		return anyFileMatch(root, "**/*_test.go")
	case KindPython:
		return dirExists(root, "tests") || dirExists(root, "test") ||
			anyFileMatch(root, "**/test_*.py") || anyFileMatch(root, "**/*_test.py")
	case KindNode:
		return dirExists(root, "test") || dirExists(root, "tests") ||
			anyFileMatch(root, "**/*.test.js") || anyFileMatch(root, "**/*.spec.ts")
	case KindRust:
		return dirExists(root, "tests") || anyFileMatch(root, "**/*_test.rs")
	case KindJava:
		return dirExists(root, "src/test/java")
	default:
		return false
	}
}

func hasDocsIndicator(root string) bool {
	return fileExists(root, "README.md") || dirExists(root, "docs")
}

func hasConfigsIndicator(root string) bool {
	patterns := []string{"**/*.yaml", "**/*.yml", "**/*.json", "**/*.toml", "**/*.ini", ".env.example"}
	for _, pat := range patterns {
		if anyFileMatch(root, pat) {
			return true
		}
	}
	return false
}

func hasInfraIndicator(root string) bool {
	markers := []string{".github", ".gitlab", "Dockerfile", "docker-compose.yml", "Makefile", "justfile"}
	for _, m := range markers {
		if _, err := os.Stat(filepath.Join(root, m)); err == nil {
			return true
		}
	}
	return false
}

// ----------------------------------------------------------------------
// Slice generation
// ----------------------------------------------------------------------

func buildSlices(proj ProjectInfo, paths []string) map[string]config.SliceConfig {
	slices := make(map[string]config.SliceConfig)

	// Language‑specific code slice
	codePatterns := codePatternsFor(proj.Kind)
	if len(codePatterns) > 0 && countMatches(paths, codePatterns) > 0 {
		slices["code"] = config.SliceConfig{
			Include:  codePatterns,
			Priority: 100,
		}
	}

	// Tests slice
	if proj.HasTests {
		testPatterns := testPatternsFor(proj.Kind)
		if len(testPatterns) > 0 && countMatches(paths, testPatterns) > 0 {
			slices["tests"] = config.SliceConfig{
				Include:  testPatterns,
				Priority: 40,
			}
		}
	}

	// Universal slices (if they match files)
	addUniversalSlice(slices, "docs", []string{"README*", "docs/**", "*.md"}, 20, paths)
	addUniversalSlice(slices, "configs", []string{
		"**/*.yaml", "**/*.yml", "**/*.json", "**/*.toml", "**/*.ini",
		".env.example", "*.config.js", "*.config.ts",
	}, 15, paths)
	addUniversalSlice(slices, "infra", []string{
		".github/**", ".gitlab/**", "Dockerfile*", "docker-compose*.yml",
		"Makefile", "justfile",
	}, 10, paths)
	addUniversalSlice(slices, "scripts", []string{
		"scripts/**", "**/*.sh", "**/*.ps1",
	}, 5, paths)

	// Special case for frontend web apps: separate components/styles/etc.
	if proj.IsWebApp {
		addUniversalSlice(slices, "components", []string{"src/components/**", "src/views/**"}, 90, paths)
		addUniversalSlice(slices, "pages", []string{"src/pages/**", "src/routes/**", "app/**", "pages/**"}, 80, paths)
		addUniversalSlice(slices, "styles", []string{"**/*.css", "**/*.scss", "**/*.less"}, 30, paths)
	}

	// Ensure at least one slice exists
	if len(slices) == 0 {
		slices["code"] = config.SliceConfig{
			Include:  []string{"**/*"},
			Priority: 100,
		}
	}

	return slices
}

func codePatternsFor(kind ProjectKind) []string {
	switch kind {
	case KindGo:
		return []string{"**/*.go", "!**/*_test.go"}
	case KindPython:
		return []string{"**/*.py", "!**/test_*.py", "!**/*_test.py", "!tests/**"}
	case KindNode:
		return []string{"**/*.js", "**/*.ts", "**/*.jsx", "**/*.tsx", "!**/*.test.*", "!**/*.spec.*"}
	case KindRust:
		return []string{"**/*.rs", "!tests/**", "!**/*_test.rs"}
	case KindJava:
		return []string{"src/main/java/**/*.java"}
	case KindRuby:
		return []string{"**/*.rb", "!test/**", "!spec/**"}
	case KindPHP:
		return []string{"**/*.php", "!tests/**"}
	case KindDotNet:
		return []string{"**/*.cs", "!**/*.Tests/**", "!**/*Test.cs"}
	default:
		// Broad set of common source extensions
		return []string{"**/*.go", "**/*.py", "**/*.js", "**/*.ts", "**/*.rs", "**/*.java", "**/*.rb", "**/*.php", "**/*.cs"}
	}
}

func testPatternsFor(kind ProjectKind) []string {
	switch kind {
	case KindGo:
		return []string{"**/*_test.go"}
	case KindPython:
		return []string{"tests/**", "test/**", "**/test_*.py", "**/*_test.py"}
	case KindNode:
		return []string{"test/**", "tests/**", "**/*.test.js", "**/*.spec.ts", "**/__tests__/**"}
	case KindRust:
		return []string{"tests/**", "**/*_test.rs"}
	case KindJava:
		return []string{"src/test/java/**"}
	case KindRuby:
		return []string{"test/**", "spec/**"}
	case KindPHP:
		return []string{"tests/**", "**/*Test.php"}
	case KindDotNet:
		return []string{"**/*.Tests/**", "**/*Test.cs"}
	default:
		return []string{"test/**", "tests/**", "**/*_test.*", "**/*.test.*", "**/*.spec.*"}
	}
}

func addUniversalSlice(m map[string]config.SliceConfig, name string, patterns []string, priority int, paths []string) {
	if countMatches(paths, patterns) > 0 {
		m[name] = config.SliceConfig{
			Include:  patterns,
			Priority: priority,
		}
	}
}

// ----------------------------------------------------------------------
// Profile generation
// ----------------------------------------------------------------------

func buildProfiles(proj ProjectInfo, slices map[string]config.SliceConfig) map[string]config.Profile {
	// Default: core code + essential universal slices
	defaultEnable := []string{}
	if _, ok := slices["code"]; ok {
		defaultEnable = append(defaultEnable, "code")
	}
	for _, s := range []string{"docs", "configs"} {
		if _, ok := slices[s]; ok {
			defaultEnable = append(defaultEnable, s)
		}
	}
	if proj.IsWebApp {
		for _, s := range []string{"components", "pages"} {
			if _, ok := slices[s]; ok && !contains(defaultEnable, s) {
				defaultEnable = append(defaultEnable, s)
			}
		}
	}
	if len(defaultEnable) == 0 {
		// fallback to first available slice
		for n := range slices {
			defaultEnable = []string{n}
			break
		}
	}

	// Full: everything
	fullEnable := make([]string, 0, len(slices))
	for n := range slices {
		fullEnable = append(fullEnable, n)
	}
	sort.Strings(fullEnable)

	// Minimal: only the core code slice
	minimalEnable := []string{}
	if _, ok := slices["code"]; ok {
		minimalEnable = []string{"code"}
	} else {
		minimalEnable = defaultEnable[:1]
	}

	// Debug: default + tests
	debugEnable := defaultEnable
	if _, ok := slices["tests"]; ok && !contains(debugEnable, "tests") {
		debugEnable = append(debugEnable, "tests")
	}

	return map[string]config.Profile{
		"default": {Enable: defaultEnable},
		"full":    {Enable: fullEnable},
		"minimal": {Enable: minimalEnable},
		"debug":   {Enable: debugEnable},
	}
}

// ----------------------------------------------------------------------
// Interactive review
// ----------------------------------------------------------------------

func interactiveReview(cfg *config.Config, slices map[string]config.SliceConfig, profiles map[string]config.Profile) error {
	in := bufio.NewReader(os.Stdin)

	fmt.Fprintln(os.Stderr, "\nDetected slices (with file matches):")
	printSliceSummary(os.Stderr, slices, profiles["default"].Enable)

	fmt.Fprintf(os.Stderr, "\nDefault profile will include: %s\n", strings.Join(profiles["default"].Enable, ", "))
	fmt.Fprintln(os.Stderr, "You can adjust using modifiers (e.g., +tests -configs). Press Enter to accept.")

	line, _ := in.ReadString('\n')
	line = strings.TrimSpace(line)
	if line != "" {
		fields := strings.Fields(line)
		mods, err := selector.ParseModifiers(fields)
		if err != nil {
			return fmt.Errorf("invalid modifiers: %w", err)
		}
		newEnable, err := applyModifiersToEnable(profiles["default"].Enable, mods, slices)
		if err != nil {
			return err
		}
		profiles["default"] = config.Profile{Enable: newEnable}
		cfg.Profiles = profiles
	}
	return nil
}

func printSliceSummary(w *os.File, slices map[string]config.SliceConfig, enabled []string) {
	type row struct {
		name     string
		enabled  bool
		priority int
	}
	var rows []row
	for name, sl := range slices {
		rows = append(rows, row{name, contains(enabled, name), sl.Priority})
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
		fmt.Fprintf(w, "  [%s] %-12s  priority=%d\n", mark, r.name, r.priority)
	}
}

// ----------------------------------------------------------------------
// Helpers
// ----------------------------------------------------------------------

func collectRepoFiles(root string) ([]string, error) {
	ignorePatterns := []string{
		".git/**", "node_modules/**", "dist/**", "build/**", ".venv/**", "venv/**",
		"__pycache__/**", "*.pyc", ".pytest_cache/**", "coverage/**", "target/**", ".snip/**",
	}
	var out []string
	skipDir := func(rel string) bool {
		relSlash := filepath.ToSlash(rel) + "/"
		for _, pat := range ignorePatterns {
			ok, _ := doublestar.Match(pat, relSlash)
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

func countMatches(paths []string, patterns []string) int {
	var c int
	for _, p := range paths {
		for _, pat := range patterns {
			ok, _ := doublestar.Match(pat, p)
			if ok {
				c++
				break
			}
		}
	}
	return c
}

func anyFileMatch(root, pattern string) bool {
	matches, _ := doublestar.FilepathGlob(filepath.Join(root, pattern))
	return len(matches) > 0
}

func fileExists(root, name string) bool {
	_, err := os.Stat(filepath.Join(root, name))
	return err == nil
}

func dirExists(root, name string) bool {
	info, err := os.Stat(filepath.Join(root, name))
	return err == nil && info.IsDir()
}

func contains(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
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
		return nil, fmt.Errorf("profile must enable at least one slice")
	}
	return out, nil
}
