package main

import (
	"fmt"
	"os"

	"github.com/mmrzaf/snip/internal/app"
	"github.com/mmrzaf/snip/internal/initwizard"
	"github.com/spf13/cobra"
)

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
			if _, err := fmt.Fprintln(os.Stdout, path); err != nil {
				return app.Wrap(app.ExitIO, fmt.Errorf("write stdout: %w", err))
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Overwrite existing .snip.yaml")
	cmd.Flags().BoolVar(&nonInteractive, "non-interactive", false, "Do not prompt")
	cmd.Flags().StringVar(&profileDefault, "profile-default", "", "Preferred default profile name (non-persistent hint)")
	return cmd
}
