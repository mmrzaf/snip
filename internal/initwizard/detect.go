package initwizard

import (
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// ProjectKind describes the primary language/runtime inferred for a project.
type ProjectKind string

const (
	// KindUnknown represents an unrecognized project kind.
	KindUnknown ProjectKind = "unknown"
	// KindGo represents a Go project.
	KindGo ProjectKind = "go"
	// KindNode represents a Node.js project.
	KindNode ProjectKind = "node"
	// KindPython represents a Python project.
	KindPython ProjectKind = "python"
	// KindRust represents a Rust project.
	KindRust ProjectKind = "rust"
	// KindJava represents a Java project.
	KindJava ProjectKind = "java"
	// KindRuby represents a Ruby project.
	KindRuby ProjectKind = "ruby"
	// KindPHP represents a PHP project.
	KindPHP ProjectKind = "php"
	// KindDotNet represents a .NET project.
	KindDotNet ProjectKind = "dotnet"
	// KindDocs represents a documentation-focused project.
	KindDocs ProjectKind = "docs"
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

func detectProject(paths []string, hint string) ProjectInfo {
	info := ProjectInfo{Kind: kindFromHint(hint)}
	if info.Kind == KindUnknown {
		info.Kind = detectLanguage(paths)
	}

	info.IsService = hasAnyPath(paths, servicePatterns(info.Kind))
	info.IsLibrary = !info.IsService && hasAnyPath(paths, libraryPatterns(info.Kind))
	info.IsWebApp = info.Kind == KindNode && hasAnyPath(paths, []string{
		"vite.config.*", "webpack.config.js", "next.config.*", "src/App.jsx", "src/App.tsx", "src/App.vue", "angular.json",
	})
	info.HasTests = hasAnyPath(paths, testPatternsFor(info.Kind))
	info.HasDocs = hasAnyPath(paths, []string{"README*", "docs/**"})
	info.HasConfigs = hasAnyPath(paths, []string{"**/*.yaml", "**/*.yml", "**/*.json", "**/*.toml", "**/*.ini", ".env.example"})
	info.HasInfra = hasAnyPath(paths, []string{".github/**", ".gitlab/**", "Dockerfile*", "docker-compose*.yml", "Makefile", "justfile"})

	return info
}

func kindFromHint(hint string) ProjectKind {
	switch strings.ToLower(strings.TrimSpace(hint)) {
	case "":
		return KindUnknown
	case "go", "golang":
		return KindGo
	case "python", "py":
		return KindPython
	case "node", "nodejs", "javascript", "typescript":
		return KindNode
	case "rust", "rs":
		return KindRust
	case "java":
		return KindJava
	case "ruby", "rb":
		return KindRuby
	case "php":
		return KindPHP
	case "dotnet", "csharp", "cs":
		return KindDotNet
	default:
		return KindUnknown
	}
}

func detectLanguage(paths []string) ProjectKind {
	for _, p := range paths {
		base := filepath.Base(p)
		switch base {
		case "go.mod":
			return KindGo
		case "requirements.txt", "setup.py", "pyproject.toml", "Pipfile":
			return KindPython
		case "package.json", "yarn.lock", "pnpm-lock.yaml":
			return KindNode
		case "Cargo.toml":
			return KindRust
		case "pom.xml", "build.gradle":
			return KindJava
		case "Gemfile":
			return KindRuby
		case "composer.json":
			return KindPHP
		}
		if strings.HasSuffix(base, ".csproj") || strings.HasSuffix(base, ".sln") {
			return KindDotNet
		}
	}
	return KindUnknown
}

func servicePatterns(kind ProjectKind) []string {
	switch kind {
	case KindGo:
		return []string{"cmd/**", "main.go"}
	case KindPython:
		return []string{"app.py", "main.py", "routers/**", "api/**"}
	case KindNode:
		return []string{"server.js", "index.js", "pages/api/**", "app/api/**"}
	case KindRust:
		return []string{"src/main.rs"}
	case KindJava:
		return []string{"src/main/java/**"}
	case KindRuby:
		return []string{"config.ru", "app/controllers/**"}
	case KindPHP:
		return []string{"public/**", "index.php"}
	case KindDotNet:
		return []string{"Program.cs", "Startup.cs"}
	default:
		return nil
	}
}

func libraryPatterns(kind ProjectKind) []string {
	switch kind {
	case KindGo:
		return []string{"pkg/**", "internal/**"}
	case KindPython:
		return []string{"src/**"}
	case KindNode:
		return []string{"index.js"}
	default:
		return nil
	}
}

func hasAnyPath(paths []string, patterns []string) bool {
	for _, p := range paths {
		for _, pat := range patterns {
			if ok, _ := doublestar.Match(pat, p); ok {
				return true
			}
		}
	}
	return false
}
