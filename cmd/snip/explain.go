package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/mmrzaf/snip/internal/app"
	"github.com/spf13/cobra"
)

func newExplainCmd(cfgPath, rootOverride *string, verbose *bool) *cobra.Command {
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
			out, err := app.Explain(context.Background(), app.ExplainOptions{
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
			if _, err := fmt.Fprint(os.Stdout, out); err != nil {
				return app.Wrap(app.ExitIO, fmt.Errorf("write stdout: %w", err))
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&profile, "profile", "", "Profile (defaults to config default_profile)")
	cmd.Flags().BoolVar(&includeHidden, "include-hidden", false, "Allow hidden files unless excluded by sensitive/ignore rules")
	cmd.Flags().SetInterspersed(false)
	return cmd
}
