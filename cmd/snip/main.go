package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mmrzaf/snip/internal/app"
	"github.com/mmrzaf/snip/internal/config"
	"github.com/mmrzaf/snip/internal/initwizard"
)

func main() {
	os.Exit(run())
}

func run() int {
	ctx := context.Background()

	var (
		cfgPath      string
		rootOverride string
		verbose      bool
	)

	rootCmd := &cobra.Command{
		Use:           "snip [profile] [modifiers...]",
		Short:         "snip bundles source context into deterministic markdown snapshots",
		SilenceUsage:  true,
		SilenceErrors: true,
		Example: strings.TrimSpace(`
# Default snapshot (uses default_profile from .snip.yaml)
snip

# Snapshot a specific profile
snip api
snip debug

# Toggle slices at runtime
snip api +tests
snip debug -docs +configs

# Print diagnostics
snip doctor
snip explain internal/app/snip.go

# Traditional subcommands still work
snip run api +tests
snip ls api
`),
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			if cmd.Name() != "init" {
				cfgPath = config.FindConfigPath(cfgPath)
			}
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			// Default behavior: run snapshot when no subcommand is specified.
			cfg, err := config.Load(cfgPath)
			if err != nil {
				return app.Wrap(app.ExitUsage, err)
			}
			profile := cfg.DefaultProfile
			mods := args
			if len(args) > 0 && !isModifier(args[0]) {
				profile = args[0]
				mods = args[1:]
			}
			_, err = app.Run(ctx, app.RunOptions{
				ConfigPath:   cfgPath,
				RootOverride: rootOverride,
				Profile:      profile,
				Modifiers:    mods,
				// Output empty => respects cfg.output.stdout_default and default file output.
				Logger: loggerFn(verbose),
			})
			return err
		},
	}

	rootCmd.PersistentFlags().StringVar(&cfgPath, "config", "", "Path to .snip.yaml (or set SNIP_CONFIG)")
	rootCmd.PersistentFlags().StringVar(&rootOverride, "root", "", "Root directory override")
	rootCmd.PersistentFlags().BoolVar(&verbose, "verbose", false, "Enable verbose output")

	rootCmd.AddCommand(newInitCmd(&rootOverride))
	rootCmd.AddCommand(newRunCmd(ctx, &cfgPath, &rootOverride, &verbose))
	rootCmd.AddCommand(newLsCmd(ctx, &cfgPath, &rootOverride, &verbose))
	rootCmd.AddCommand(newDoctorCmd(ctx, &cfgPath, &rootOverride, &verbose))
	rootCmd.AddCommand(newExplainCmd(ctx, &cfgPath, &rootOverride, &verbose))
	rootCmd.AddCommand(newVersionCmd())

	if err := rootCmd.Execute(); err != nil {
		code := app.ExitIO
		var ae *app.Error
		if errors.As(err, &ae) {
			code = ae.ExitCode()
		}
		fmt.Fprintln(os.Stderr, "error:", err)
		return code
	}
	return app.ExitOK
}

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

func newInitCmd(rootOverride *string) *cobra.Command {
	var (
		force          bool
		nonInteractive bool
		profileDefault string
	)
	cmd := &cobra.Command{
		Use:     "init",
		Short:   "Create a .snip.yaml configuration file",
		Example: "snip init\nsnip init --non-interactive\n",
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := initwizard.Run(initwizard.Options{
				Root:           *rootOverride,
				Force:          force,
				NonInteractive: nonInteractive,
				ProfileDefault: profileDefault,
			})
			if err != nil {
				return app.Wrap(app.ExitIO, err)
			}
			fmt.Fprintln(os.Stdout, path)
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "Overwrite existing .snip.yaml")
	cmd.Flags().BoolVar(&nonInteractive, "non-interactive", false, "Do not prompt")
	cmd.Flags().StringVar(&profileDefault, "profile-default", "", "Preferred default profile name (non-persistent hint)")
	return cmd
}

func newRunCmd(ctx context.Context, cfgPath *string, rootOverride *string, verbose *bool) *cobra.Command {
	var (
		out           string
		stdout        bool
		maxChars      int
		format        string
		noTree        bool
		noManifest    bool
		treeDepth     int
		includeHidden bool
		quiet         bool
	)
	cmd := &cobra.Command{
		Use:   "run <profile> [modifiers...]",
		Short: "Generate a bundle for a profile",
		Args:  cobra.MinimumNArgs(1),
		Example: strings.TrimSpace(`
snip run api
snip run api +tests
snip run debug --stdout
snip run api -docs --max-chars 200000
`),
		RunE: func(cmd *cobra.Command, args []string) error {
			profile := args[0]
			mods := args[1:]
			effectiveOut := out
			if stdout {
				effectiveOut = "-"
			}
			res, err := app.Run(ctx, app.RunOptions{
				ConfigPath:    *cfgPath,
				RootOverride:  *rootOverride,
				Profile:       profile,
				Modifiers:     mods,
				Output:        effectiveOut,
				MaxChars:      maxChars,
				Format:        format,
				NoTree:        noTree,
				NoManifest:    noManifest,
				TreeDepth:     treeDepth,
				IncludeHidden: includeHidden,
				Logger:        loggerFn(*verbose),
			})
			if !quiet && res.OutputPath != "" && res.OutputPath != "-" {
				fmt.Fprintln(os.Stdout, res.OutputPath)
			}
			return err
		},
	}
	cmd.Flags().StringVarP(&out, "out", "o", "", "Output file path override ('-' for stdout)")
	cmd.Flags().BoolVar(&stdout, "stdout", false, "Write to stdout (equivalent to -o -)")
	cmd.Flags().IntVar(&maxChars, "max-chars", 0, "Override budgets.max_chars")
	cmd.Flags().StringVar(&format, "format", "md", "Output format (md)")
	cmd.Flags().BoolVar(&noTree, "no-tree", false, "Disable tree section")
	cmd.Flags().BoolVar(&noManifest, "no-manifest", false, "Disable manifest sections")
	cmd.Flags().IntVar(&treeDepth, "tree-depth", 0, "Override render.tree_depth")
	cmd.Flags().BoolVar(&includeHidden, "include-hidden", false, "Allow hidden files unless excluded by sensitive/ignore rules")
	cmd.Flags().BoolVar(&quiet, "quiet", false, "Do not print output path")
	return cmd
}

func newLsCmd(ctx context.Context, cfgPath *string, rootOverride *string, verbose *bool) *cobra.Command {
	var (
		maxChars      int
		includeHidden bool
	)
	cmd := &cobra.Command{
		Use:   "ls <profile> [modifiers...]",
		Short: "List files that would be included",
		Args:  cobra.MinimumNArgs(1),
		Example: strings.TrimSpace(`
snip ls api
snip ls api +tests
snip ls debug -docs
`),
		RunE: func(cmd *cobra.Command, args []string) error {
			profile := args[0]
			mods := args[1:]
			out, _, err := app.List(ctx, app.ListOptions{
				ConfigPath:    *cfgPath,
				RootOverride:  *rootOverride,
				Profile:       profile,
				Modifiers:     mods,
				MaxChars:      maxChars,
				IncludeHidden: includeHidden,
				Verbose:       *verbose,
				Logger:        loggerFn(*verbose),
			})
			if out != "" {
				fmt.Fprint(os.Stdout, out)
			}
			return err
		},
	}
	cmd.Flags().IntVar(&maxChars, "max-chars", 0, "Override budgets.max_chars")
	cmd.Flags().BoolVar(&includeHidden, "include-hidden", false, "Allow hidden files unless excluded by sensitive/ignore rules")
	return cmd
}

func newDoctorCmd(ctx context.Context, cfgPath *string, rootOverride *string, verbose *bool) *cobra.Command {
	var (
		profile       string
		includeHidden bool
	)
	cmd := &cobra.Command{
		Use:   "doctor [modifiers...]",
		Short: "Print effective config + environment diagnostics",
		Example: strings.TrimSpace(`
snip doctor
snip doctor +tests
snip doctor --profile debug -docs
`),
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := app.Doctor(ctx, app.DoctorOptions{
				ConfigPath:    *cfgPath,
				RootOverride:  *rootOverride,
				Profile:       profile,
				Modifiers:     args,
				IncludeHidden: includeHidden,
				Logger:        loggerFn(*verbose),
			})
			if err != nil {
				return err
			}
			fmt.Fprint(os.Stdout, out)
			return nil
		},
	}
	cmd.Flags().StringVar(&profile, "profile", "", "Profile (defaults to config default_profile)")
	cmd.Flags().BoolVar(&includeHidden, "include-hidden", false, "Allow hidden files unless excluded by sensitive/ignore rules")
	return cmd
}

func newExplainCmd(ctx context.Context, cfgPath *string, rootOverride *string, verbose *bool) *cobra.Command {
	var (
		profile       string
		includeHidden bool
	)
	cmd := &cobra.Command{
		Use:   "explain <path> [modifiers...]",
		Short: "Explain why a path is included/excluded and what matched",
		Args:  cobra.MinimumNArgs(1),
		Example: strings.TrimSpace(`
snip explain internal/app/snip.go
snip explain .github/workflows/ci.yml
snip explain internal/app/snip.go +tests
`),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := args[0]
			mods := args[1:]
			out, err := app.Explain(ctx, app.ExplainOptions{
				ConfigPath:    *cfgPath,
				RootOverride:  *rootOverride,
				Profile:       profile,
				Modifiers:     mods,
				IncludeHidden: includeHidden,
				Path:          target,
				Logger:        loggerFn(*verbose),
			})
			if err != nil {
				return err
			}
			fmt.Fprint(os.Stdout, out)
			return nil
		},
	}
	cmd.Flags().StringVar(&profile, "profile", "", "Profile (defaults to config default_profile)")
	cmd.Flags().BoolVar(&includeHidden, "include-hidden", false, "Allow hidden files unless excluded by sensitive/ignore rules")
	return cmd
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "version",
		Short:   "Print version",
		Example: "snip version\n",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprintln(os.Stdout, app.Version)
		},
	}
}

// small utility for stable output ordering
func sortedKeys(m map[string]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
