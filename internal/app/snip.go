package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mmrzaf/snip/internal/budget"
	"github.com/mmrzaf/snip/internal/config"
	"github.com/mmrzaf/snip/internal/discovery"
	"github.com/mmrzaf/snip/internal/gitinfo"
	"github.com/mmrzaf/snip/internal/render"
	"github.com/mmrzaf/snip/internal/selector"
	"github.com/mmrzaf/snip/internal/util"
)

// RunOptions configures snip run.
type RunOptions struct {
	ConfigPath    string
	RootOverride  string
	Profile       string
	Modifiers     []string
	Output        string // "-" for stdout
	MaxChars      int
	Format        string
	NoTree        bool
	NoManifest    bool
	TreeDepth     int
	IncludeHidden bool
	Logger        *slog.Logger
	Now           func() time.Time
}

// RunResult is the result of snip run.
type RunResult struct {
	OutputPath string
	Partial    bool
	HardCut    bool
}

// Run executes a snapshot run and writes output.
func Run(ctx context.Context, opts RunOptions) (RunResult, error) {
	log := opts.Logger
	if log == nil {
		log = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Format != "" && opts.Format != "md" {
		return RunResult{}, Wrap(ExitUsage, fmt.Errorf("unsupported format %q", opts.Format))
	}
	cfg, err := config.Load(opts.ConfigPath)
	if err != nil {
		return RunResult{}, Wrap(ExitUsage, err)
	}
	root, err := config.EffectiveRoot(cfg, opts.RootOverride)
	if err != nil {
		return RunResult{}, Wrap(ExitUsage, err)
	}
	cfg, err = config.ApplyProfileOverrides(cfg, opts.Profile)
	if err != nil {
		return RunResult{}, Wrap(ExitUsage, err)
	}

	mods, err := selector.ParseModifiers(opts.Modifiers)
	if err != nil {
		return RunResult{}, Wrap(ExitUsage, err)
	}
	enabled, err := selector.EnabledSlices(cfg, opts.Profile, mods)
	if err != nil {
		return RunResult{}, Wrap(ExitUsage, err)
	}
	enabledOrdered := selector.EnabledSliceList(enabled, cfg)

	limits := budget.Limits{
		MaxChars:        cfg.Budgets.MaxChars,
		PerFileMaxLines: cfg.Budgets.PerFileMaxLines,
		PerFileMaxBytes: cfg.Budgets.PerFileMaxBytes,
	}
	if opts.MaxChars > 0 {
		limits.MaxChars = opts.MaxChars
	}

	renderCfg := cfg.Render
	if opts.NoTree {
		renderCfg.IncludeTree = false
	}
	if opts.NoManifest {
		renderCfg.IncludeManifest = false
	}
	if opts.TreeDepth > 0 {
		renderCfg.TreeDepth = opts.TreeDepth
	}

	slicePriorities := map[string]int{}
	for _, s := range enabled {
		slicePriorities[s] = cfg.Slices[s].Priority
	}

	eng, err := discovery.NewEngine(root, cfg.Ignore.UseGitignore, cfg.Ignore.Always, cfg.Sensitive.ExcludeGlobs, cfg.Ignore.BinaryExtensions)
	if err != nil {
		return RunResult{}, Wrap(ExitIO, err)
	}
	discovered, err := eng.Discover()
	if err != nil {
		return RunResult{}, Wrap(ExitIO, err)
	}
	log.Debug("discovered files", "count", len(discovered))

	selected, err := selector.Select(cfg, enabled, discovered, opts.IncludeHidden)
	if err != nil {
		return RunResult{}, Wrap(ExitUsage, err)
	}
	log.Debug("selected", "included", len(selected.Included), "dropped", len(selected.Dropped))

	b := &budget.Builder{Limits: limits}
	plan, err := b.BuildPlan(ctx, opts.Profile, enabledOrdered, selected)
	if err != nil {
		return RunResult{}, Wrap(ExitIO, err)
	}

	sha, err := gitinfo.ShortSHA(ctx, root)
	if err != nil || sha == "" {
		sha = "000000"
	}

	rndr := render.Renderer{
		Newline:         renderCfg.Newline,
		CodeFences:      renderCfg.CodeFences,
		IncludeTree:     renderCfg.IncludeTree,
		TreeDepth:       renderCfg.TreeDepth,
		IncludeManifest: renderCfg.IncludeManifest,
		Manifest: render.ManifestOptions{
			GroupBySlice:           renderCfg.Manifest.GroupBySlice,
			IncludeLineCounts:      renderCfg.Manifest.IncludeLineCounts,
			IncludeByteCounts:      renderCfg.Manifest.IncludeByteCounts,
			IncludeTruncationNotes: renderCfg.Manifest.IncludeTruncationNotes,
			IncludeUnreadableNotes: renderCfg.Manifest.IncludeUnreadableNotes,
		},
		FileBlock: render.FileBlockOptions{
			Header: renderCfg.FileBlock.Header,
			Footer: renderCfg.FileBlock.Footer,
		},
	}

	rootLabel := cfg.Root
	if opts.RootOverride != "" {
		rootLabel = opts.RootOverride
	}
	if rootLabel == "" {
		rootLabel = "."
	}

	now := opts.Now().In(time.Local)
	info := render.BundleInfo{
		Repo:        filepath.Base(root),
		Root:        rootLabel,
		Profile:     opts.Profile,
		Enabled:     enabledOrdered,
		GitSHA:      sha,
		Timestamp:   now,
		SnipVersion: Version,
	}

	renderFn := func(p budget.Plan) (string, error) { return rndr.RenderMarkdown(info, p) }
	planFinal, rendered, err := b.EnforceGlobalBudget(ctx, plan, slicePriorities, renderFn)
	if err != nil {
		return RunResult{}, Wrap(ExitIO, err)
	}

	warnPartial(os.Stderr, planFinal)

	stdout := opts.Output == "-" || (opts.Output == "" && cfg.Output.StdoutDefault)
	if stdout {
		if _, err := fmt.Fprint(os.Stdout, rendered); err != nil {
			return RunResult{}, Wrap(ExitIO, fmt.Errorf("write stdout: %w", err))
		}
		res := RunResult{OutputPath: "-", Partial: planFinal.Partial, HardCut: planFinal.HardCut}
		if res.Partial {
			return res, Wrap(ExitPartial, fmt.Errorf("partial output"))
		}
		return res, nil
	}

	if opts.Output != "" {
		outPath, err := writeExplicitOutput(opts.Output, rendered)
		if err != nil {
			return RunResult{}, Wrap(ExitIO, err)
		}
		res := RunResult{OutputPath: outPath, Partial: planFinal.Partial, HardCut: planFinal.HardCut}
		if res.Partial {
			return res, Wrap(ExitPartial, fmt.Errorf("partial output"))
		}
		return res, nil
	}

	outPath, err := writeDefaultOutput(root, cfg, opts.Profile, sha, now, rendered)
	if err != nil {
		return RunResult{}, Wrap(ExitIO, err)
	}
	res := RunResult{OutputPath: outPath, Partial: planFinal.Partial, HardCut: planFinal.HardCut}
	if res.Partial {
		return res, Wrap(ExitPartial, fmt.Errorf("partial output"))
	}
	return res, nil
}

func warnPartial(w *os.File, plan budget.Plan) {
	warn := func(msg string) {
		_, _ = fmt.Fprintln(w, "warning:", msg)
	}
	for _, s := range plan.DroppedSlices {
		warn(fmt.Sprintf("slice dropped due to budget: %s", s))
	}
	for _, d := range plan.Dropped {
		switch d.Reason {
		case "unreadable":
			warn(fmt.Sprintf("unreadable file excluded: %s", d.RelPath))
		case "invalid_utf8":
			warn(fmt.Sprintf("invalid UTF-8 file excluded: %s", d.RelPath))
		case "budget_exceeded":
			// Files are already implied by slice warnings; keep noise low.
		}
	}
}

// ListOptions configures snip ls.
type ListOptions struct {
	ConfigPath    string
	RootOverride  string
	Profile       string
	Modifiers     []string
	MaxChars      int
	IncludeHidden bool
	Verbose       bool
	Logger        *slog.Logger
	Now           func() time.Time
}

// List executes the selection and budget enforcement and prints a dry-run listing.
func List(ctx context.Context, opts ListOptions) (string, bool, error) {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	log := opts.Logger
	if log == nil {
		log = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	}

	cfg, err := config.Load(opts.ConfigPath)
	if err != nil {
		return "", false, Wrap(ExitUsage, err)
	}
	root, err := config.EffectiveRoot(cfg, opts.RootOverride)
	if err != nil {
		return "", false, Wrap(ExitUsage, err)
	}
	cfg, err = config.ApplyProfileOverrides(cfg, opts.Profile)
	if err != nil {
		return "", false, Wrap(ExitUsage, err)
	}
	mods, err := selector.ParseModifiers(opts.Modifiers)
	if err != nil {
		return "", false, Wrap(ExitUsage, err)
	}
	enabled, err := selector.EnabledSlices(cfg, opts.Profile, mods)
	if err != nil {
		return "", false, Wrap(ExitUsage, err)
	}
	enabledOrdered := selector.EnabledSliceList(enabled, cfg)

	limits := budget.Limits{
		MaxChars:        cfg.Budgets.MaxChars,
		PerFileMaxLines: cfg.Budgets.PerFileMaxLines,
		PerFileMaxBytes: cfg.Budgets.PerFileMaxBytes,
	}
	if opts.MaxChars > 0 {
		limits.MaxChars = opts.MaxChars
	}
	b := &budget.Builder{Limits: limits}

	slicePriorities := map[string]int{}
	for _, s := range enabled {
		slicePriorities[s] = cfg.Slices[s].Priority
	}

	eng, err := discovery.NewEngine(root, cfg.Ignore.UseGitignore, cfg.Ignore.Always, cfg.Sensitive.ExcludeGlobs, cfg.Ignore.BinaryExtensions)
	if err != nil {
		return "", false, Wrap(ExitIO, err)
	}
	discovered, err := eng.Discover()
	if err != nil {
		return "", false, Wrap(ExitIO, err)
	}
	selected, err := selector.Select(cfg, enabled, discovered, opts.IncludeHidden)
	if err != nil {
		return "", false, Wrap(ExitUsage, err)
	}

	plan, err := b.BuildPlan(ctx, opts.Profile, enabledOrdered, selected)
	if err != nil {
		return "", false, Wrap(ExitIO, err)
	}

	sha, err := gitinfo.ShortSHA(ctx, root)
	if err != nil || sha == "" {
		sha = "000000"
	}
	rndr := render.Renderer{
		Newline:         cfg.Render.Newline,
		CodeFences:      cfg.Render.CodeFences,
		IncludeTree:     cfg.Render.IncludeTree,
		TreeDepth:       cfg.Render.TreeDepth,
		IncludeManifest: cfg.Render.IncludeManifest,
		Manifest: render.ManifestOptions{
			GroupBySlice:           cfg.Render.Manifest.GroupBySlice,
			IncludeLineCounts:      cfg.Render.Manifest.IncludeLineCounts,
			IncludeByteCounts:      cfg.Render.Manifest.IncludeByteCounts,
			IncludeTruncationNotes: cfg.Render.Manifest.IncludeTruncationNotes,
			IncludeUnreadableNotes: cfg.Render.Manifest.IncludeUnreadableNotes,
		},
		FileBlock: render.FileBlockOptions{
			Header: cfg.Render.FileBlock.Header,
			Footer: cfg.Render.FileBlock.Footer,
		},
	}

	rootLabel := cfg.Root
	if opts.RootOverride != "" {
		rootLabel = opts.RootOverride
	}
	if rootLabel == "" {
		rootLabel = "."
	}

	now := opts.Now().In(time.Local)
	info := render.BundleInfo{
		Repo:        filepath.Base(root),
		Root:        rootLabel,
		Profile:     opts.Profile,
		Enabled:     enabledOrdered,
		GitSHA:      sha,
		Timestamp:   now,
		SnipVersion: Version,
	}
	renderFn := func(p budget.Plan) (string, error) { return rndr.RenderMarkdown(info, p) }
	planFinal, _, err := b.EnforceGlobalBudget(ctx, plan, slicePriorities, renderFn)
	if err != nil {
		return "", false, Wrap(ExitIO, err)
	}

	log.Debug("ls finalized", "included", len(planFinal.Included), "dropped", len(planFinal.Dropped), "partial", planFinal.Partial)

	var sb strings.Builder
	sb.WriteString("Enabled slices: [" + strings.Join(enabledOrdered, ", ") + "]\n")
	sb.WriteString("Included files:\n")
	for i, f := range planFinal.Included {
		fmt.Fprintf(&sb, "  %3d  %s  slices=[%s] primary=%s truncated=%t\n", i+1, f.RelPath, strings.Join(f.Slices, ","), f.PrimarySlice, f.Truncated)
	}

	if len(planFinal.DroppedSlices) > 0 {
		for _, s := range planFinal.DroppedSlices {
			fmt.Fprintf(&sb, "Dropped slice due to budget: %s\n", s)
		}
	}

	if opts.Verbose {
		sb.WriteString("Dropped:\n")
		for _, d := range planFinal.Dropped {
			fmt.Fprintf(&sb, "  - %s reason=%s", d.RelPath, d.Reason)
			if d.Detail != "" {
				fmt.Fprintf(&sb, " detail=%s", d.Detail)
			}
			if d.PrimarySlice != "" {
				fmt.Fprintf(&sb, " slice=%s", d.PrimarySlice)
			}
			sb.WriteString("\n")
		}
	} else {
		var droppedCount int
		for _, d := range planFinal.Dropped {
			if d.Reason == "budget_exceeded" {
				droppedCount++
			}
		}
		if droppedCount > 0 {
			fmt.Fprintf(&sb, "Dropped files due to budget: %d (use --verbose for details)\n", droppedCount)
		}
	}

	partial := planFinal.Partial
	if partial {
		return sb.String(), true, Wrap(ExitPartial, fmt.Errorf("partial output"))
	}
	return sb.String(), false, nil
}

func writeExplicitOutput(path string, rendered string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("out path is empty")
	}
	if path == "-" {
		return "-", nil
	}
	if !filepath.IsAbs(path) {
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("getwd: %w", err)
		}
		path = filepath.Join(cwd, path)
	}
	path = filepath.Clean(path)
	if err := util.AtomicWriteFile(path, []byte(rendered), 0o644); err != nil {
		return "", fmt.Errorf("write bundle: %w", err)
	}
	return path, nil
}

func writeDefaultOutput(root string, cfg config.Config, profile string, gitsha string, ts time.Time, rendered string) (string, error) {
	outDir := cfg.Output.Dir
	if outDir == "" {
		outDir = ".snip"
	}

	var absDir string
	if filepath.IsAbs(outDir) {
		absDir = filepath.Clean(outDir)
	} else {
		absDir = filepath.Clean(filepath.Join(root, outDir))
	}

	if err := os.MkdirAll(absDir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir output dir: %w", err)
	}

	tokens := map[string]string{
		"ts":      ts.Format("20060102-150405"),
		"profile": profile,
		"gitsha":  gitsha,
		"repo":    filepath.Base(root),
		"name":    filepath.Base(root),
	}

	if strings.Contains(cfg.Output.Pattern, "{counter}") {
		c, err := util.NextCounter(absDir)
		if err != nil {
			return "", fmt.Errorf("counter: %w", err)
		}
		tokens["counter"] = fmt.Sprintf("%03d", c)
	}

	fileName := util.ApplyPatternTokens(cfg.Output.Pattern, tokens)
	fileName = strings.ReplaceAll(fileName, "/", "_")
	fileName = strings.ReplaceAll(fileName, "\\", "_")
	fileName = filepath.Base(fileName)
	if !strings.HasSuffix(strings.ToLower(fileName), ".md") {
		fileName += ".md"
	}

	outPath := filepath.Join(absDir, fileName)
	if err := util.AtomicWriteFile(outPath, []byte(rendered), 0o644); err != nil {
		return "", fmt.Errorf("write bundle: %w", err)
	}

	if cfg.Output.Latest != "" {
		latestName := filepath.Base(cfg.Output.Latest)
		latestPath := filepath.Join(absDir, latestName)
		if err := util.AtomicWriteFile(latestPath, []byte(rendered), 0o644); err != nil {
			return "", fmt.Errorf("write latest: %w", err)
		}
	}

	return outPath, nil
}
