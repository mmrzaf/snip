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

	create, overwrite, blocked := 0, 0, 0
	for _, f := range r.Files {
		switch {
		case f.Blocked:
			blocked++
		case f.Exists:
			overwrite++
		default:
			create++
		}
	}
	fmt.Fprintf(&b, "%d file(s) (%d create, %d overwrite, %d blocked)", len(r.Files), create, overwrite, blocked)
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
		if f.Blocked {
			action = "BLOCKED (exists, use --force with --write)"
		} else if f.Overwrite {
			action = "OVERWRITE"
		} else if f.Exists {
			action = "WOULD OVERWRITE"
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
		} else {
			fmt.Fprintf(&b, "  CREATED %s\n", f.RelPath)
		}
	}
	return b.String()
}
