package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/mmrzaf/snip/internal/app"
	"github.com/mmrzaf/snip/internal/initwizard"
	"github.com/spf13/cobra"
)

func newInitCmd(rootOverride *string) *cobra.Command {
	var (
		force          bool
		interactive    bool
		nonInteractive bool
		profileDefault string
	)

	cmd := &cobra.Command{
		Use:     "init",
		Short:   "Create a .snip.yaml configuration file",
		Example: "snip init\nsnip init --interactive\nsnip init --force\n",
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := initwizard.Run(initwizard.Options{
				Root:           *rootOverride,
				Force:          force,
				Interactive:    interactive && !nonInteractive,
				NonInteractive: true,
				ProfileDefault: profileDefault,
			})
			if err != nil {
				if errors.Is(err, initwizard.ErrConfigExists) ||
					errors.Is(err, initwizard.ErrInvalidProfileDefault) ||
					errors.Is(err, initwizard.ErrInvalidInteractiveInput) {
					return app.Wrap(app.ExitUsage, err)
				}
				return app.Wrap(app.ExitIO, err)
			}
			if _, err := fmt.Fprintf(os.Stdout, "created %s\n", path); err != nil {
				return app.Wrap(app.ExitIO, fmt.Errorf("write stdout: %w", err))
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Overwrite existing .snip.yaml")
	cmd.Flags().BoolVar(&interactive, "interactive", false, "Review detected slices before writing")
	cmd.Flags().BoolVar(&nonInteractive, "non-interactive", false, "Compatibility alias; init is non-interactive by default")
	cmd.Flags().StringVar(&profileDefault, "profile-default", "", "Preferred default profile name")
	return cmd
}
