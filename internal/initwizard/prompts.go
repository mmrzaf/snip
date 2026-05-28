package initwizard

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/mmrzaf/snip/internal/config"
	"github.com/mmrzaf/snip/internal/selector"
)

func interactiveReview(cfg *config.Config, slices map[string]config.SliceConfig, profiles map[string]config.Profile) error {
	in := bufio.NewReader(os.Stdin)

	fmt.Fprintln(os.Stderr, "\nDetected slices:")
	printSliceSummary(os.Stderr, slices, profiles["api"].Enable)

	fmt.Fprintf(os.Stderr, "\nDefault profile is %q and includes: %s\n", cfg.DefaultProfile, strings.Join(profiles["api"].Enable, ", "))
	fmt.Fprintln(os.Stderr, "Optional modifiers for api profile (example: +tests -configs). Press Enter to accept:")

	line, err := in.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("%w: %v", ErrInvalidInteractiveInput, err)
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return nil
	}

	fields := strings.Fields(line)
	mods, err := selector.ParseModifiers(fields)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidInteractiveInput, err)
	}
	newEnable, err := applyModifiersToEnable(profiles["api"].Enable, mods, slices)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidInteractiveInput, err)
	}
	profiles["api"] = config.Profile{Enable: newEnable}
	cfg.Profiles = profiles
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
	if len(out) == 0 {
		return nil, errors.New("profile must enable at least one slice")
	}
	return out, nil
}
