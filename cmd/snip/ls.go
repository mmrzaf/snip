package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/mmrzaf/snip/internal/app"
	"github.com/spf13/cobra"
)

func newLsCmd(cfgPath, rootOverride *string, verbose *bool) *cobra.Command {
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
			out, _, err := app.List(context.Background(), app.ListOptions{
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
				if _, err := fmt.Fprint(os.Stdout, out); err != nil {
					return app.Wrap(app.ExitIO, fmt.Errorf("write stdout: %w", err))
				}
			}
			return err
		},
	}

	cmd.Flags().IntVar(&maxChars, "max-chars", 0, "Override budgets.max_chars")
	cmd.Flags().BoolVar(&includeHidden, "include-hidden", false, "Allow hidden files unless excluded by sensitive/ignore rules")
	return cmd
}
