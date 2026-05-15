package main

import (
	"os"

	"github.com/mmrzaf/snip/internal/app"
)

func main() {
	os.Exit(run())
}

// run executes the root command and returns the exit code.
func run() int {
	rootCmd := newRootCommand()
	rootCmd.SetArgs(preprocessCLIArgs(os.Args[1:]))

	if err := rootCmd.Execute(); err != nil {
		code := app.ExitIO
		var ae *app.Error
		if as, ok := err.(*app.Error); ok {
			ae = as
		}
		if ae != nil {
			code = ae.ExitCode()
		}
		// Error already printed by cobra (SilenceErrors is false here),
		// but we need to ensure stderr gets it. Cobra prints to stderr automatically.
		return code
	}
	return app.ExitOK
}
