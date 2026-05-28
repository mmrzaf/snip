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
	return strings.HasPrefix(s, "+") || reDashModifier.MatchString(s)
}

// preprocessCLIArgs escapes dash-prefixed slice modifiers before cobra parses flags.
// Snip treats "-docs" as a domain modifier, not a CLI flag, once positional args begin.
func preprocessCLIArgs(args []string) []string {
	if len(args) == 0 {
		return args
	}

	cmdIdx := firstCommandIndex(args)
	if cmdIdx < 0 {
		return escapeDefaultCommandDashModifiers(args)
	}

	switch args[cmdIdx] {
	case "run", "ls":
		return escapeAfterNPositionals(args, cmdIdx+1, 1)
	case "doctor":
		return escapeAfterNPositionals(args, cmdIdx+1, 0)
	case "explain":
		return escapeAfterNPositionals(args, cmdIdx+1, 1)
	default:
		return args
	}
}

func firstCommandIndex(args []string) int {
	expectValue := false
	for i, a := range args {
		if expectValue {
			expectValue = false
			continue
		}
		if a == "--" {
			return -1
		}
		if needsValue, ok := isGlobalFlag(a); ok {
			expectValue = needsValue
			continue
		}
		if strings.HasPrefix(a, "-") {
			continue
		}
		switch a {
		case "init", "run", "ls", "doctor", "explain", "apply", "version":
			return i
		default:
			return -1
		}
	}
	return -1
}

func escapeDefaultCommandDashModifiers(args []string) []string {
	out := make([]string, 0, len(args))
	expectValue := false
	sawProfile := false

	for _, a := range args {
		if expectValue {
			out = append(out, a)
			expectValue = false
			continue
		}
		if a == "--" {
			out = append(out, a)
			continue
		}
		if needsValue, ok := isGlobalFlag(a); ok {
			out = append(out, a)
			expectValue = needsValue
			continue
		}

		if isDashModifierToken(a) {
			out = append(out, escapedModifierPrefix+a)
			continue
		}

		if !sawProfile && !strings.HasPrefix(a, "-") {
			sawProfile = true
		}
		out = append(out, a)
	}
	return out
}

func escapeAfterNPositionals(args []string, start int, n int) []string {
	out := make([]string, 0, len(args))
	out = append(out, args[:start]...)

	expectValue := false
	positionals := 0
	for i := start; i < len(args); i++ {
		a := args[i]

		if expectValue {
			out = append(out, a)
			expectValue = false
			continue
		}
		if a == "--" {
			out = append(out, args[i:]...)
			break
		}

		if needsValue, ok := isKnownFlag(a); ok {
			out = append(out, a)
			expectValue = needsValue
			continue
		}

		if positionals >= n && isDashModifierToken(a) {
			out = append(out, escapedModifierPrefix+a)
			continue
		}

		if !strings.HasPrefix(a, "-") {
			positionals++
		}
		out = append(out, a)
	}
	return out
}

func isDashModifierToken(arg string) bool {
	return reDashModifier.MatchString(arg)
}

func isGlobalFlag(arg string) (needsValue bool, ok bool) {
	switch arg {
	case "--config", "--root":
		return true, true
	case "--verbose":
		return false, true
	}
	if strings.HasPrefix(arg, "--config=") || strings.HasPrefix(arg, "--root=") {
		return false, true
	}
	return false, false
}

func isKnownFlag(arg string) (needsValue bool, ok bool) {
	if needsValue, ok := isGlobalFlag(arg); ok {
		return needsValue, ok
	}
	switch arg {
	case "-o", "--out", "--max-chars", "--format", "--tree-depth", "--profile", "--file-header":
		return true, true
	case "--stdout", "--no-tree", "--no-manifest", "--include-hidden", "--quiet", "--force", "--write", "--interactive", "--non-interactive":
		return false, true
	}
	for _, prefix := range []string{
		"--out=", "--max-chars=", "--format=", "--tree-depth=", "--profile=", "--file-header=",
	} {
		if strings.HasPrefix(arg, prefix) {
			return false, true
		}
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
