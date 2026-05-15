package main

import (
	"context"
	"strings"

	"github.com/mmrzaf/snip/internal/app"
	"github.com/mmrzaf/snip/internal/config"
	"github.com/spf13/cobra"
)

// rootFlags holds persistent flags for the root command.
type rootFlags struct {
	cfgPath      string
	rootOverride string
	verbose      bool
}

func newRootCommand() *cobra.Command {
	var flags rootFlags

	cmd := &cobra.Command{
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
				flags.cfgPath = config.FindConfigPath(flags.cfgPath)
			}
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			// Default behavior: run snapshot when no subcommand is specified.
			cfg, err := config.Load(flags.cfgPath)
			if err != nil {
				return app.Wrap(app.ExitUsage, err)
			}
			profile := cfg.DefaultProfile
			mods := args
			if len(args) > 0 && !isModifier(args[0]) {
				profile = args[0]
				mods = args[1:]
			}
			_, err = app.Run(context.Background(), app.RunOptions{
				ConfigPath:   flags.cfgPath,
				RootOverride: flags.rootOverride,
				Profile:      profile,
				Modifiers:    mods,
				Logger:       loggerFn(flags.verbose),
			})
			return err
		},
	}

	cmd.PersistentFlags().StringVar(&flags.cfgPath, "config", "", "Path to .snip.yaml (or set SNIP_CONFIG)")
	cmd.PersistentFlags().StringVar(&flags.rootOverride, "root", "", "Root directory override")
	cmd.PersistentFlags().BoolVar(&flags.verbose, "verbose", false, "Enable verbose output")

	// Add subcommands
	cmd.AddCommand(newInitCmd(&flags.rootOverride))
	cmd.AddCommand(newRunCmd(&flags.cfgPath, &flags.rootOverride, &flags.verbose))
	cmd.AddCommand(newLsCmd(&flags.cfgPath, &flags.rootOverride, &flags.verbose))
	cmd.AddCommand(newDoctorCmd(&flags.cfgPath, &flags.rootOverride, &flags.verbose))
	cmd.AddCommand(newExplainCmd(&flags.cfgPath, &flags.rootOverride, &flags.verbose))
	cmd.AddCommand(newApplyCmd(&flags.rootOverride))
	cmd.AddCommand(newVersionCmd())

	return cmd
}
