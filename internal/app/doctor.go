package app

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mmrzaf/snip/internal/budget"
	"github.com/mmrzaf/snip/internal/config"
	"github.com/mmrzaf/snip/internal/discovery"
	"github.com/mmrzaf/snip/internal/gitinfo"
	"github.com/mmrzaf/snip/internal/selector"
)

// DoctorOptions configures snip doctor.
type DoctorOptions struct {
	ConfigPath    string
	RootOverride  string
	Profile       string
	Modifiers     []string
	IncludeHidden bool
	Logger        *slog.Logger
	Now           func() time.Time
}

// Doctor returns effective configuration and environment diagnostics.
func Doctor(ctx context.Context, opts DoctorOptions) (string, error) {
	cfg, err := config.Load(opts.ConfigPath)
	if err != nil {
		return "", Wrap(ExitUsage, err)
	}
	root, err := config.EffectiveRoot(cfg, opts.RootOverride)
	if err != nil {
		return "", Wrap(ExitUsage, err)
	}

	profile := opts.Profile
	if profile == "" {
		profile = cfg.DefaultProfile
	}

	cfg, err = config.ApplyProfileOverrides(cfg, profile)
	if err != nil {
		return "", Wrap(ExitUsage, err)
	}

	mods, err := selector.ParseModifiers(opts.Modifiers)
	if err != nil {
		return "", Wrap(ExitUsage, err)
	}
	enabled, err := selector.EnabledSlices(cfg, profile, mods)
	if err != nil {
		return "", Wrap(ExitUsage, err)
	}
	enabledOrdered := selector.EnabledSliceList(enabled, cfg)

	limits := budget.Limits{
		MaxChars:        cfg.Budgets.MaxChars,
		PerFileMaxLines: cfg.Budgets.PerFileMaxLines,
		PerFileMaxBytes: cfg.Budgets.PerFileMaxBytes,
	}

	sha, shaErr := gitinfo.ShortSHA(ctx, root)
	gitAvail := shaErr == nil && sha != ""
	if !gitAvail {
		sha = "(unavailable)"
	}

	eng, err := discovery.NewEngine(root, cfg.Ignore.UseGitignore, cfg.Ignore.Always, cfg.Sensitive.ExcludeGlobs, cfg.Ignore.BinaryExtensions)
	if err != nil {
		return "", Wrap(ExitIO, err)
	}
	discovered, err := eng.Discover()
	if err != nil {
		return "", Wrap(ExitIO, err)
	}
	sel, err := selector.Select(cfg, enabled, discovered, opts.IncludeHidden)
	if err != nil {
		return "", Wrap(ExitUsage, err)
	}

	reasonCounts := map[string]int{}
	for _, d := range sel.Dropped {
		reasonCounts[string(d.ExclusionReason)]++
	}
	type kv struct {
		k string
		v int
	}
	var rows []kv
	for k, v := range reasonCounts {
		rows = append(rows, kv{k: k, v: v})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].v != rows[j].v {
			return rows[i].v > rows[j].v
		}
		return rows[i].k < rows[j].k
	})
	if len(rows) > 8 {
		rows = rows[:8]
	}

	var b strings.Builder
	w := func(s string, a ...any) { fmt.Fprintf(&b, s+"\n", a...) }

	w("snip doctor")
	w("")
	w("config_path: %s", opts.ConfigPath)
	w("root: %s", filepath.Clean(root))
	w("profile: %s", profile)
	w("enabled_slices: [%s]", strings.Join(enabledOrdered, ", "))
	w("budgets: max_chars=%d per_file_max_lines=%d per_file_max_bytes=%d", limits.MaxChars, limits.PerFileMaxLines, limits.PerFileMaxBytes)
	w("git: available=%t sha=%s", gitAvail, sha)
	w("discovery: use_gitignore=%t include_hidden=%t", cfg.Ignore.UseGitignore, opts.IncludeHidden)

	w("")
	w("top_exclusion_reasons:")
	if len(rows) == 0 {
		w("  (none)")
	} else {
		for _, r := range rows {
			w("  - %s: %d", r.k, r.v)
		}
	}

	return b.String(), nil
}

// ExplainOptions configures snip explain.
type ExplainOptions struct {
	ConfigPath    string
	RootOverride  string
	Profile       string
	Modifiers     []string
	IncludeHidden bool
	Path          string
	Logger        *slog.Logger
	Now           func() time.Time
}

// Explain returns inclusion/exclusion details for a single path.
func Explain(ctx context.Context, opts ExplainOptions) (string, error) {
	cfg, err := config.Load(opts.ConfigPath)
	if err != nil {
		return "", Wrap(ExitUsage, err)
	}
	root, err := config.EffectiveRoot(cfg, opts.RootOverride)
	if err != nil {
		return "", Wrap(ExitUsage, err)
	}

	profile := opts.Profile
	if profile == "" {
		profile = cfg.DefaultProfile
	}

	cfg, err = config.ApplyProfileOverrides(cfg, profile)
	if err != nil {
		return "", Wrap(ExitUsage, err)
	}

	mods, err := selector.ParseModifiers(opts.Modifiers)
	if err != nil {
		return "", Wrap(ExitUsage, err)
	}
	enabled, err := selector.EnabledSlices(cfg, profile, mods)
	if err != nil {
		return "", Wrap(ExitUsage, err)
	}
	enabledOrdered := selector.EnabledSliceList(enabled, cfg)

	// Normalize path to relpath under root (best effort).
	rel := opts.Path
	if filepath.IsAbs(rel) {
		if r, rerr := filepath.Rel(root, rel); rerr == nil {
			rel = r
		}
	}
	rel = filepath.ToSlash(filepath.Clean(rel))
	rel = strings.TrimPrefix(rel, "./")

	eng, err := discovery.NewEngine(root, cfg.Ignore.UseGitignore, cfg.Ignore.Always, cfg.Sensitive.ExcludeGlobs, cfg.Ignore.BinaryExtensions)
	if err != nil {
		return "", Wrap(ExitIO, err)
	}
	discovered, err := eng.Discover()
	if err != nil {
		return "", Wrap(ExitIO, err)
	}

	var pi *discovery.PathInfo
	for i := range discovered {
		if discovered[i].RelPath == rel {
			pi = &discovered[i]
			break
		}
	}

	var b strings.Builder
	w := func(s string, a ...any) { fmt.Fprintf(&b, s+"\n", a...) }

	w("snip explain")
	w("")
	w("path: %s", rel)
	w("root: %s", filepath.Clean(root))
	w("profile: %s", profile)
	w("enabled_slices: [%s]", strings.Join(enabledOrdered, ", "))
	w("include_hidden: %t", opts.IncludeHidden)

	if pi == nil {
		w("")
		w("discovery: not_found_under_root=true")
		return b.String(), nil
	}

	w("")
	w("discovery:")
	w("  excluded: %t", pi.Excluded)
	if pi.Excluded {
		w("  reason: %s", string(pi.ExclusionReason))
		w("  detail: %s", pi.ExclusionDetail)
	} else {
		w("  reason: (none)")
	}

	// Slice match detail (for all slices, but highlight enabled ones).
	type sm struct {
		name             string
		priority         int
		includeMatched   bool
		includePattern   string
		includeExplicitH bool
		excludeMatched   bool
		excludePattern   string
		member           bool
	}
	var matches []sm
	for name, sl := range cfg.Slices {
		inc, incPat, incExplicitHidden, exc, excPat := selector.ExplainSliceMatch(rel, sl)
		member := inc && !exc
		matches = append(matches, sm{
			name:             name,
			priority:         sl.Priority,
			includeMatched:   inc,
			includePattern:   incPat,
			includeExplicitH: incExplicitHidden,
			excludeMatched:   exc,
			excludePattern:   excPat,
			member:           member,
		})
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].priority != matches[j].priority {
			return matches[i].priority > matches[j].priority
		}
		return matches[i].name < matches[j].name
	})

	enabledSet := map[string]bool{}
	for _, s := range enabled {
		enabledSet[s] = true
	}

	// Effective membership under enabled slices + hidden policy (mirrors selector.membership logic).
	var effective []string
	isHidden := pi.IsHidden
	for _, m := range matches {
		if !enabledSet[m.name] {
			continue
		}
		if !m.member {
			continue
		}
		if isHidden && !opts.IncludeHidden && !m.includeExplicitH {
			continue
		}
		effective = append(effective, m.name)
	}
	sort.Strings(effective)

	w("")
	w("slice_matches:")
	for _, m := range matches {
		if !m.includeMatched && !m.excludeMatched {
			continue
		}
		tag := " "
		if enabledSet[m.name] {
			tag = "x"
		}
		w("  [%s] %s (priority=%d)", tag, m.name, m.priority)
		if m.includeMatched {
			w("      include: matched pattern=%q", m.includePattern)
		}
		if m.excludeMatched {
			w("      exclude: matched pattern=%q", m.excludePattern)
		}
	}

	w("")
	w("effective_selection:")
	w("  in_enabled_slices: %t", len(effective) > 0)
	w("  matched_enabled_slices: [%s]", strings.Join(effective, ", "))
	w("  included: %t", !pi.Excluded && len(effective) > 0)

	return b.String(), nil
}
