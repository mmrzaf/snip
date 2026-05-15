package main

import (
	"fmt"
	"os"

	"github.com/mmrzaf/snip/internal/app"
	"github.com/spf13/cobra"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "version",
		Short:   "Print version",
		Example: "snip version\n",
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := fmt.Fprintln(os.Stdout, app.Version); err != nil {
				return app.Wrap(app.ExitIO, fmt.Errorf("write stdout: %w", err))
			}
			return nil
		},
	}
}
