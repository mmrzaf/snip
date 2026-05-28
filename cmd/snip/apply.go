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
		Short: "Apply markdown file blocks to the filesystem",
		Long: strings.TrimSpace(`
Apply markdown file blocks to the filesystem.

Default mode is a safe dry-run. Use --write to write files.
When --file-header is omitted, snip auto-detects common Snip headers:
  <<<FILE:{path}>>>
  ===== FILE: {path} =====
`),
		Args: cobra.ExactArgs(1),
		Example: strings.TrimSpace(`
snip apply bundle.md
snip apply bundle.md --write
snip apply bundle.md --write --force
snip apply ai.txt --file-header '===== FILE: {path} ====='
`),
		RunE: func(cmd *cobra.Command, args []string) error {
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
				if _, err := fmt.Fprintf(os.Stdout, "%s\n", res.PlanSummary()); err != nil {
					return app.Wrap(app.ExitIO, fmt.Errorf("write stdout: %w", err))
				}
				if _, err := fmt.Fprint(os.Stdout, res.VerbosePlan()); err != nil {
					return app.Wrap(app.ExitIO, fmt.Errorf("write stdout: %w", err))
				}
				return nil
			}

			if _, err := fmt.Fprint(os.Stdout, res.WriteSummary()); err != nil {
				return app.Wrap(app.ExitIO, fmt.Errorf("write stdout: %w", err))
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&fileHeader, "file-header", "", "Optional header line template containing {path}")
	cmd.Flags().BoolVar(&write, "write", false, "Write files to disk (default is dry-run)")
	cmd.Flags().BoolVar(&force, "force", false, "Allow overwriting existing files in write mode")
	return cmd
}
