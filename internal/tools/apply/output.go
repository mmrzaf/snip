package apply

import (
	"fmt"
	"strings"
)

// PlanSummary returns a human-readable summary of the planned operations.
func (r Result) PlanSummary() string {
	var b strings.Builder
	if r.DryRun {
		b.WriteString("DRY-RUN plan: ")
	} else {
		b.WriteString("Applied: ")
	}
	create := 0
	overwrite := 0
	for _, f := range r.Files {
		if f.Exists {
			overwrite++
		} else {
			create++
		}
	}
	fmt.Fprintf(&b, "%d file(s) (%d create, %d overwrite)", len(r.Files), create, overwrite)
	return b.String()
}

// VerbosePlan returns a detailed list of planned changes, suitable for dry-run.
func (r Result) VerbosePlan() string {
	if len(r.Files) == 0 {
		return "No files to write.\n"
	}
	var b strings.Builder
	for _, f := range r.Files {
		action := "CREATE"
		if f.Exists {
			if f.Overwrite {
				action = "OVERWRITE"
			} else {
				action = "SKIP (exists, use --force)"
			}
		}
		fmt.Fprintf(&b, "%s %s (%d bytes)\n", action, f.RelPath, len(f.Content))
	}
	return b.String()
}

// WriteSummary returns a summary after writing files.
func (r Result) WriteSummary() string {
	if r.Wrote == 0 {
		return "No files written.\n"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Wrote %d file(s):\n", r.Wrote)
	for _, f := range r.Files {
		if f.Overwrite {
			fmt.Fprintf(&b, "  OVERWROTE %s\n", f.RelPath)
		} else if !f.Exists {
			fmt.Fprintf(&b, "  CREATED %s\n", f.RelPath)
		}
	}
	return b.String()
}
