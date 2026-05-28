package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/mmrzaf/snip/internal/app"
)

func main() {
	os.Exit(run())
}

// run executes the root command and returns the process exit code.
func run() int {
	rootCmd := newRootCommand()
	rootCmd.SetArgs(preprocessCLIArgs(os.Args[1:]))

	if err := rootCmd.Execute(); err != nil {
		code := app.ExitIO
		var ae *app.Error
		if errors.As(err, &ae) {
			code = ae.ExitCode()
		}
		_, _ = fmt.Fprintln(os.Stderr, "error:", err)
		return code
	}
	return app.ExitOK
}
