package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/mmrzaf/snip/internal/app"
	"github.com/spf13/cobra"
)

func newDoctorCmd(cfgPath, rootOverride *string, verbose *bool) *cobra.Command {
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
			args = unescapeModifiers(args)
			out, err := app.Doctor(context.Background(), app.DoctorOptions{
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
			if _, err := fmt.Fprint(os.Stdout, out); err != nil {
				return app.Wrap(app.ExitIO, fmt.Errorf("write stdout: %w", err))
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&profile, "profile", "", "Profile (defaults to config default_profile)")
	cmd.Flags().BoolVar(&includeHidden, "include-hidden", false, "Allow hidden files unless excluded by sensitive/ignore rules")
	return cmd
}
