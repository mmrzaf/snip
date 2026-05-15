package initwizard

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/mmrzaf/snip/internal/config"
	"github.com/mmrzaf/snip/internal/selector"
)

func interactiveReview(cfg *config.Config, slices map[string]config.SliceConfig, profiles map[string]config.Profile) error {
	in := bufio.NewReader(os.Stdin)

	fmt.Fprintln(os.Stderr, "\nDetected slices (with file matches):")
	printSliceSummary(os.Stderr, slices, profiles["default"].Enable)

	fmt.Fprintf(os.Stderr, "\nDefault profile will include: %s\n", strings.Join(profiles["default"].Enable, ", "))
	fmt.Fprintln(os.Stderr, "You can adjust using modifiers (e.g., +tests -configs). Press Enter to accept.")

	line, _ := in.ReadString('\n')
	line = strings.TrimSpace(line)
	if line != "" {
		fields := strings.Fields(line)
		mods, err := selector.ParseModifiers(fields)
		if err != nil {
			return fmt.Errorf("invalid modifiers: %w", err)
		}
		newEnable, err := applyModifiersToEnable(profiles["default"].Enable, mods, slices)
		if err != nil {
			return err
		}
		profiles["default"] = config.Profile{Enable: newEnable}
		cfg.Profiles = profiles
	}
	return nil
}

func printSliceSummary(w *os.File, slices map[string]config.SliceConfig, enabled []string) {
	type row struct {
		name     string
		enabled  bool
		priority int
	}
	var rows []row
	for name, sl := range slices {
		rows = append(rows, row{name, contains(enabled, name), sl.Priority})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].priority != rows[j].priority {
			return rows[i].priority > rows[j].priority
		}
		return rows[i].name < rows[j].name
	})
	for _, r := range rows {
		mark := " "
		if r.enabled {
			mark = "x"
		}
		_, _ = fmt.Fprintf(w, "  [%s] %-12s  priority=%d\n", mark, r.name, r.priority)
	}
}

func applyModifiersToEnable(current []string, mods []selector.Modifier, slices map[string]config.SliceConfig) ([]string, error) {
	enabled := map[string]bool{}
	for _, s := range current {
		enabled[s] = true
	}
	for _, m := range mods {
		if _, ok := slices[m.Name]; !ok {
			return nil, fmt.Errorf("unknown slice %q", m.Name)
		}
		enabled[m.Name] = m.Enable
	}
	var out []string
	for s, on := range enabled {
		if on {
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out, nil
}
