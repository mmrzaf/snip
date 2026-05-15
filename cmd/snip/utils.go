package main

import (
	"log/slog"
	"os"
	"regexp"
	"strings"
)

const escapedModifierPrefix = "__snip_modifier__:"

var reDashModifier = regexp.MustCompile(`^-[A-Za-z0-9][A-Za-z0-9_-]*$`)

func loggerFn(verbose bool) *slog.Logger {
	lvl := slog.LevelInfo
	if verbose {
		lvl = slog.LevelDebug
	}
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lvl}))
}

func isModifier(s string) bool {
	return strings.HasPrefix(s, "+") || strings.HasPrefix(s, "-")
}

func preprocessCLIArgs(args []string) []string {
	if len(args) < 3 || args[0] != "run" {
		return args
	}
	out := make([]string, 0, len(args))
	out = append(out, args[0])
	out = append(out, escapeRunDashModifiers(args[1:])...)
	return out
}

func escapeRunDashModifiers(args []string) []string {
	out := make([]string, 0, len(args))
	sawProfile := false
	expectValue := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			out = append(out, args[i:]...)
			break
		}
		if expectValue {
			out = append(out, a)
			expectValue = false
			continue
		}
		if needsValue, ok := isRunFlag(a); ok {
			out = append(out, a)
			if needsValue {
				expectValue = true
			}
			continue
		}
		if !sawProfile {
			if strings.HasPrefix(a, "-") {
				out = append(out, a)
				continue
			}
			sawProfile = true
			out = append(out, a)
			continue
		}
		if reDashModifier.MatchString(a) {
			out = append(out, escapedModifierPrefix+a)
			continue
		}
		out = append(out, a)
	}
	return out
}

func isRunFlag(arg string) (needsValue bool, ok bool) {
	switch arg {
	case "-o", "--out", "--max-chars", "--format", "--tree-depth", "--config", "--root":
		return true, true
	case "--stdout", "--no-tree", "--no-manifest", "--include-hidden", "--quiet", "--verbose":
		return false, true
	}
	if strings.HasPrefix(arg, "--out=") ||
		strings.HasPrefix(arg, "--max-chars=") ||
		strings.HasPrefix(arg, "--format=") ||
		strings.HasPrefix(arg, "--tree-depth=") ||
		strings.HasPrefix(arg, "--config=") ||
		strings.HasPrefix(arg, "--root=") {
		return false, true
	}
	if strings.HasPrefix(arg, "-o") && len(arg) > 2 {
		return false, true
	}
	return false, false
}

func unescapeModifiers(args []string) []string {
	out := make([]string, 0, len(args))
	for _, a := range args {
		if strings.HasPrefix(a, escapedModifierPrefix) {
			raw := strings.TrimPrefix(a, escapedModifierPrefix)
			if reDashModifier.MatchString(raw) {
				out = append(out, raw)
				continue
			}
		}
		out = append(out, a)
	}
	return out
}
