package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/mmrzaf/snip/internal/app"
	applytool "github.com/mmrzaf/snip/internal/tools/apply"
	"github.com/spf13/cobra"
)

func newApplyCmd(rootOverride *string) *cobra.Command {
	var (
		fileHeader string
		write      bool
		force      bool
	)

	cmd := &cobra.Command{
		Use:   "apply <input-file>",
		Short: "Apply AI-generated markdown code blocks to the filesystem",
		Long: strings.TrimSpace(`
Apply AI-generated markdown code blocks to the filesystem.
Does not require snip format.
`),
		Args: cobra.ExactArgs(1),
		Example: strings.TrimSpace(`
snip apply ai.txt --file-header '===== FILE: {path} ====='
snip apply ai.txt --file-header '<<<FILE:{path}>>>' --write --force
`),
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(fileHeader) == "" {
				return app.Wrap(app.ExitUsage, fmt.Errorf("--file-header is required (must contain {path})"))
			}
			res, err := applytool.Run(args[0], applytool.Options{
				Root:       *rootOverride,
				FileHeader: fileHeader,
				Write:      write,
				Force:      force,
			})
			if err != nil {
				if applytool.IsKind(err, applytool.KindInvalidInput) {
					return app.Wrap(app.ExitUsage, err)
				}
				if applytool.IsKind(err, applytool.KindIO) {
					return app.Wrap(app.ExitIO, err)
				}
				return app.Wrap(app.ExitIO, err)
			}

			if !write {
				// Dry-run: show detailed plan
				if _, err := fmt.Fprintf(os.Stdout, "%s\n", res.PlanSummary()); err != nil {
					return app.Wrap(app.ExitIO, fmt.Errorf("write stdout: %w", err))
				}
				if _, err := fmt.Fprint(os.Stdout, res.VerbosePlan()); err != nil {
					return app.Wrap(app.ExitIO, fmt.Errorf("write stdout: %w", err))
				}
				return nil
			}

			// Write mode: show summary
			if _, err := fmt.Fprint(os.Stdout, res.WriteSummary()); err != nil {
				return app.Wrap(app.ExitIO, fmt.Errorf("write stdout: %w", err))
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&fileHeader, "file-header", "", "Header line template containing {path} (e.g. '===== FILE: {path} =====')")
	cmd.Flags().BoolVar(&write, "write", false, "Write files to disk (default is dry-run)")
	cmd.Flags().BoolVar(&force, "force", false, "Allow overwriting existing files")
	return cmd
}
