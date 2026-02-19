package selector

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/mmrzaf/snip/internal/config"
	"github.com/mmrzaf/snip/internal/discovery"
)

var reSliceName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

// Modifier represents a run-time slice toggle.
type Modifier struct {
	Name   string
	Enable bool
}

// ParseModifiers parses CLI modifiers like +tests or -docs.
func ParseModifiers(args []string) ([]Modifier, error) {
	mods := make([]Modifier, 0, len(args))
	for _, a := range args {
		if a == "" {
			continue
		}
		if a[0] != '+' && a[0] != '-' {
			return nil, fmt.Errorf("invalid modifier %q", a)
		}
		name := a[1:]
		if !reSliceName.MatchString(name) {
			return nil, fmt.Errorf("invalid slice name %q", name)
		}
		mods = append(mods, Modifier{Name: name, Enable: a[0] == '+'})
	}
	return mods, nil
}

// EnabledSlices resolves enabled slices for a profile plus modifiers.
func EnabledSlices(cfg config.Config, profile string, mods []Modifier) ([]string, error) {
	p, ok := cfg.Profiles[profile]
	if !ok {
		return nil, fmt.Errorf("unknown profile %q", profile)
	}
	enabled := map[string]bool{}
	for _, s := range p.Enable {
		if _, ok := cfg.Slices[s]; !ok {
			return nil, fmt.Errorf("profile %q enables unknown slice %q", profile, s)
		}
		enabled[s] = true
	}
	for _, m := range mods {
		if _, ok := cfg.Slices[m.Name]; !ok {
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

// File describes a file considered by selection.
type File struct {
	RelPath         string
	AbsPath         string
	SizeBytes       int64
	IsHidden        bool
	Slices          []string
	PrimarySlice    string
	PrimaryPriority int
	Excluded        bool
	ExclusionReason discovery.ExclusionReason
	ExclusionDetail string
}

// Selected is the output of Select.
type Selected struct {
	Included []File
	Dropped  []File
}

// Select assigns slice membership and splits included vs dropped.
func Select(cfg config.Config, enabledSlices []string, discovered []discovery.PathInfo, includeHidden bool) (Selected, error) {
	if len(enabledSlices) == 0 {
		return Selected{}, errors.New("no enabled slices")
	}

	// Precompute slice order by priority for deterministic primary selection.
	slicePriorities := map[string]int{}
	for _, s := range enabledSlices {
		slicePriorities[s] = cfg.Slices[s].Priority
	}

	var included []File
	var dropped []File

	for _, pi := range discovered {
		mem := membership(cfg, enabledSlices, pi.RelPath, pi.IsHidden, includeHidden)
		if len(mem) == 0 {
			continue
		}

		f := File{
			RelPath:         pi.RelPath,
			AbsPath:         pi.AbsPath,
			SizeBytes:       pi.SizeBytes,
			IsHidden:        pi.IsHidden,
			Slices:          mem,
			Excluded:        pi.Excluded,
			ExclusionReason: pi.ExclusionReason,
			ExclusionDetail: pi.ExclusionDetail,
		}
		f.PrimarySlice, f.PrimaryPriority = primary(mem, slicePriorities)
		if f.Excluded {
			dropped = append(dropped, f)
			continue
		}
		included = append(included, f)
	}

	// Base ordering is stable lexicographic. Rendering may re-order based on grouping settings.
	sort.Slice(included, func(i, j int) bool { return included[i].RelPath < included[j].RelPath })
	sort.Slice(dropped, func(i, j int) bool { return dropped[i].RelPath < dropped[j].RelPath })

	return Selected{Included: included, Dropped: dropped}, nil
}

func membership(cfg config.Config, enabled []string, rel string, isHidden bool, includeHidden bool) []string {
	var mem []string
	for _, s := range enabled {
		sl := cfg.Slices[s]
		inc, incExplicitHidden := matchesAny(rel, sl.Include)
		if !inc {
			continue
		}
		if isHidden && !includeHidden && !incExplicitHidden {
			continue
		}
		if ok, _ := matchesAny(rel, sl.Exclude); ok {
			continue
		}
		mem = append(mem, s)
	}
	sort.Strings(mem)
	return mem
}

func matchesAny(rel string, patterns []string) (matched bool, explicitHidden bool) {
	for _, pat := range patterns {
		if pat == "" {
			continue
		}
		ok, err := doublestar.Match(pat, rel)
		if err != nil {
			continue
		}
		if ok {
			matched = true
			if patternExplicitlyIncludesHidden(pat) {
				explicitHidden = true
			}
		}
	}
	return matched, explicitHidden
}

func patternExplicitlyIncludesHidden(pat string) bool {
	pat = strings.ReplaceAll(pat, "\\", "/")
	for _, seg := range strings.Split(pat, "/") {
		if seg == "" {
			continue
		}
		// Consider patterns like `.github/**`, `**/.github/**`, `.env*`, `**/.env*` explicit.
		if strings.HasPrefix(seg, ".") && seg != "." && seg != ".." {
			return true
		}
	}
	return false
}

func primary(mem []string, pri map[string]int) (string, int) {
	best := ""
	bestP := -1 << 30
	for _, s := range mem {
		p := pri[s]
		if p > bestP || (p == bestP && s < best) {
			best = s
			bestP = p
		}
	}
	return best, bestP
}

// EnabledSliceList formats enabled slices deterministically, ordered by priority desc then name.
func EnabledSliceList(enabled []string, cfg config.Config) []string {
	out := append([]string(nil), enabled...)
	sort.Slice(out, func(i, j int) bool {
		pi := cfg.Slices[out[i]].Priority
		pj := cfg.Slices[out[j]].Priority
		if pi != pj {
			return pi > pj
		}
		return strings.Compare(out[i], out[j]) < 0
	})
	return out
}

// ExplainSliceMatch reports include/exclude matching details for a single slice.
// It returns:
//   - includeMatched, includePattern, includeExplicitHidden
//   - excludeMatched, excludePattern
func ExplainSliceMatch(rel string, sl config.SliceConfig) (bool, string, bool, bool, string) {
	incOK, incPat, incExplicitHidden := firstMatch(rel, sl.Include)
	excOK, excPat, _ := firstMatch(rel, sl.Exclude)
	return incOK, incPat, incExplicitHidden, excOK, excPat
}

func firstMatch(rel string, patterns []string) (matched bool, pattern string, explicitHidden bool) {
	for _, pat := range patterns {
		if pat == "" {
			continue
		}
		ok, err := doublestar.Match(pat, rel)
		if err != nil {
			continue
		}
		if ok {
			return true, pat, patternExplicitlyIncludesHidden(pat)
		}
	}
	return false, "", false
}
