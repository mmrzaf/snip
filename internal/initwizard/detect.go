package initwizard

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// ProjectKind describes the primary language/runtime inferred for a project.
type ProjectKind string

const (
	// KindUnknown is used when no language markers are detected.
	KindUnknown ProjectKind = "unknown"
	// KindGo represents Go projects.
	KindGo ProjectKind = "go"
	// KindPython represents Python projects.
	KindPython ProjectKind = "python"
	// KindNode represents Node.js projects.
	KindNode ProjectKind = "node"
	// KindRust represents Rust projects.
	KindRust ProjectKind = "rust"
	// KindJava represents Java projects.
	KindJava ProjectKind = "java"
	// KindRuby represents Ruby projects.
	KindRuby ProjectKind = "ruby"
	// KindPHP represents PHP projects.
	KindPHP ProjectKind = "php"
	// KindDotNet represents .NET projects.
	KindDotNet ProjectKind = "dotnet"
)

// ProjectInfo summarizes inferred project characteristics.
type ProjectInfo struct {
	Kind       ProjectKind
	IsService  bool
	IsLibrary  bool
	IsWebApp   bool
	HasTests   bool
	HasDocs    bool
	HasConfigs bool
	HasInfra   bool
}

// detectProject examines the root directory and returns a ProjectInfo summary.
func detectProject(root string, hint string) ProjectInfo {
	info := ProjectInfo{Kind: KindUnknown}

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

	info.IsService = hasMainIndicator(root, info.Kind)
	info.IsLibrary = !info.IsService && hasLibraryIndicator(root, info.Kind)
	info.IsWebApp = isWebApp(root, info.Kind)
	info.HasTests = hasTestsIndicator(root, info.Kind)
	info.HasDocs = hasDocsIndicator(root)
	info.HasConfigs = hasConfigsIndicator(root)
	info.HasInfra = hasInfraIndicator(root)

	return info
}

// detectLanguage returns the project kind by looking for well-known marker files.
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
	if matches, _ := filepath.Glob(filepath.Join(root, "*.csproj")); len(matches) > 0 {
		return KindDotNet
	}
	if matches, _ := filepath.Glob(filepath.Join(root, "*.sln")); len(matches) > 0 {
		return KindDotNet
	}
	return KindUnknown
}

// hasMainIndicator reports whether the project appears to be a service/executable.
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

// hasLibraryIndicator reports whether the project appears to be a library.
func hasLibraryIndicator(root string, kind ProjectKind) bool {
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

// isWebApp checks for typical web application markers (Node.js only).
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

// hasTestsIndicator reports whether the project contains tests.
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

// hasDocsIndicator reports whether the project contains documentation.
func hasDocsIndicator(root string) bool {
	return fileExists(root, "README.md") || dirExists(root, "docs")
}

// hasConfigsIndicator reports whether the project contains configuration files.
func hasConfigsIndicator(root string) bool {
	patterns := []string{"**/*.yaml", "**/*.yml", "**/*.json", "**/*.toml", "**/*.ini", ".env.example"}
	for _, pat := range patterns {
		if anyFileMatch(root, pat) {
			return true
		}
	}
	return false
}

// hasInfraIndicator reports whether the project contains infrastructure files.
func hasInfraIndicator(root string) bool {
	markers := []string{".github", ".gitlab", "Dockerfile", "docker-compose.yml", "Makefile", "justfile"}
	for _, m := range markers {
		if _, err := os.Stat(filepath.Join(root, m)); err == nil {
			return true
		}
	}
	return false
}

// fileExists returns true if the named file exists under root.
func fileExists(root, name string) bool {
	_, err := os.Stat(filepath.Join(root, name))
	return err == nil
}

// dirExists returns true if the named directory exists under root.
func dirExists(root, name string) bool {
	info, err := os.Stat(filepath.Join(root, name))
	return err == nil && info.IsDir()
}

// anyFileMatch returns true if at least one file under root matches the pattern (supports **).
func anyFileMatch(root, pattern string) bool {
	found := false
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		if ok, _ := doublestar.Match(pattern, rel); ok {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	return found
}
