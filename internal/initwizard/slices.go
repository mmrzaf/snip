package initwizard

import (
	"github.com/bmatcuk/doublestar/v4"
	"github.com/mmrzaf/snip/internal/config"
)

func buildSlices(proj ProjectInfo, paths []string) map[string]config.SliceConfig {
	slices := make(map[string]config.SliceConfig)

	codePatterns := codePatternsFor(proj.Kind)
	if len(codePatterns) > 0 && countMatches(paths, codePatterns) > 0 {
		slices["code"] = config.SliceConfig{
			Include:  codePatternsFor(proj.Kind),
			Exclude:  codeExcludePatternsFor(proj.Kind),
			Priority: 100,
		}
	}

	if proj.HasTests {
		testPatterns := testPatternsFor(proj.Kind)
		if len(testPatterns) > 0 && countMatches(paths, testPatterns) > 0 {
			slices["tests"] = config.SliceConfig{
				Include:  testPatterns,
				Priority: 40,
			}
		}
	}

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

	if proj.IsWebApp {
		addUniversalSlice(slices, "components", []string{"src/components/**", "src/views/**"}, 90, paths)
		addUniversalSlice(slices, "pages", []string{"src/pages/**", "src/routes/**", "app/**", "pages/**"}, 80, paths)
		addUniversalSlice(slices, "styles", []string{"**/*.css", "**/*.scss", "**/*.less"}, 30, paths)
	}

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
		return []string{"**/*.go"}
	case KindPython:
		return []string{"**/*.py"}
	case KindNode:
		return []string{"**/*.js", "**/*.ts", "**/*.jsx", "**/*.tsx"}
	case KindRust:
		return []string{"**/*.rs"}
	case KindJava:
		return []string{"src/main/java/**/*.java"}
	case KindRuby:
		return []string{"**/*.rb"}
	case KindPHP:
		return []string{"**/*.php"}
	case KindDotNet:
		return []string{"**/*.cs"}
	default:
		return []string{"**/*.go", "**/*.py", "**/*.js", "**/*.ts", "**/*.rs", "**/*.java", "**/*.rb", "**/*.php", "**/*.cs"}
	}
}

func codeExcludePatternsFor(kind ProjectKind) []string {
	switch kind {
	case KindGo:
		return []string{"**/*_test.go"}
	case KindPython:
		return []string{"**/test_*.py", "**/*_test.py", "tests/**"}
	case KindNode:
		return []string{"**/*.test.*", "**/*.spec.*"}
	case KindRust:
		return []string{"tests/**", "**/*_test.rs"}
	case KindJava:
		return []string{"src/test/java/**"}
	case KindRuby:
		return []string{"test/**", "spec/**"}
	case KindPHP:
		return []string{"tests/**"}
	case KindDotNet:
		return []string{"**/*.Tests/**", "**/*Test.cs"}
	default:
		return []string{"test/**", "tests/**", "**/*_test.*", "**/*.test.*", "**/*.spec.*"}
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
