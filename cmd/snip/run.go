package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/mmrzaf/snip/internal/app"
	"github.com/spf13/cobra"
)

func newRunCmd(cfgPath, rootOverride *string, verbose *bool) *cobra.Command {
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
			args = unescapeModifiers(args)
			profile := args[0]
			mods := args[1:]
			effectiveOut := out
			if stdout {
				effectiveOut = "-"
			}
			res, err := app.Run(context.Background(), app.RunOptions{
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
				if _, err := fmt.Fprintln(os.Stdout, res.OutputPath); err != nil {
					return app.Wrap(app.ExitIO, fmt.Errorf("write stdout: %w", err))
				}
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
