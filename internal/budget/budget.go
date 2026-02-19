package budget

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"unicode/utf8"

	"github.com/mmrzaf/snip/internal/selector"
	"github.com/mmrzaf/snip/internal/util"
)

var errInvalidUTF8 = errors.New("invalid utf-8")

// Limits control bundle budgets.
type Limits struct {
	MaxChars        int
	PerFileMaxLines int
	PerFileMaxBytes int
}

// Builder constructs plans and enforces budgets.
type Builder struct {
	Limits Limits
}

// FileEntry is an included file with metadata and (possibly truncated) content.
type FileEntry struct {
	RelPath       string
	AbsPath       string
	Slices        []string
	PrimarySlice  string
	Priority      int
	OriginalLines int
	OriginalBytes int64
	KeptLines     int
	KeptBytes     int
	Truncated     bool
	Content       string
}

// DroppedEntry records a dropped/excluded file.
type DroppedEntry struct {
	RelPath      string
	Slices       []string
	PrimarySlice string
	Reason       string
	Detail       string
}

// Plan represents a bundle plan.
type Plan struct {
	Profile       string
	EnabledSlices []string
	Included      []FileEntry
	Dropped       []DroppedEntry
	DroppedSlices []string
	Partial       bool
	HardCut       bool
}

// BuildPlan reads included files, applies per-file truncation, and records dropped.
func (b *Builder) BuildPlan(ctx context.Context, profile string, enabledOrdered []string, selected selector.Selected) (Plan, error) {
	p := Plan{Profile: profile, EnabledSlices: append([]string(nil), enabledOrdered...)}

	// Record excluded candidates that belong to enabled slices.
	for _, d := range selected.Dropped {
		p.Dropped = append(p.Dropped, DroppedEntry{
			RelPath:      d.RelPath,
			Slices:       append([]string(nil), d.Slices...),
			PrimarySlice: d.PrimarySlice,
			Reason:       string(d.ExclusionReason),
			Detail:       d.ExclusionDetail,
		})
		if d.ExclusionReason == "unreadable" {
			p.Partial = true
		}
	}

	for _, f := range selected.Included {
		if err := ctx.Err(); err != nil {
			return Plan{}, err
		}
		entry, err := readAndTruncateFile(f.RelPath, f.AbsPath, f.Slices, f.PrimarySlice, f.PrimaryPriority, b.Limits.PerFileMaxLines, b.Limits.PerFileMaxBytes)
		if err != nil {
			if errors.Is(err, errInvalidUTF8) {
				p.Dropped = append(p.Dropped, DroppedEntry{
					RelPath:      f.RelPath,
					Slices:       append([]string(nil), f.Slices...),
					PrimarySlice: f.PrimarySlice,
					Reason:       "invalid_utf8",
					Detail:       "invalid utf-8",
				})
				p.Partial = true
				continue
			}
			// Any read/stat error is treated as an unreadable file: continue and mark partial.
			p.Dropped = append(p.Dropped, DroppedEntry{
				RelPath:      f.RelPath,
				Slices:       append([]string(nil), f.Slices...),
				PrimarySlice: f.PrimarySlice,
				Reason:       "unreadable",
				Detail:       err.Error(),
			})
			p.Partial = true
			continue
		}
		p.Included = append(p.Included, entry)
	}

	orderPlan(&p)
	return p, nil
}

// EnforceGlobalBudget ensures the rendered plan stays under MaxChars.
// It applies the drop_low_priority policy and deterministic truncation tightening.
func (b *Builder) EnforceGlobalBudget(
	ctx context.Context,
	plan Plan,
	slicePriorities map[string]int,
	renderFn func(Plan) (string, error),
) (Plan, string, error) {
	if err := ctx.Err(); err != nil {
		return Plan{}, "", err
	}
	rendered, err := renderFn(plan)
	if err != nil {
		return Plan{}, "", err
	}
	if runeCount(rendered) <= b.Limits.MaxChars {
		return plan, rendered, nil
	}

	// Drop slices from lowest priority to highest.
	plan2 := plan
	plan2.Partial = true
	plan2.DroppedSlices = nil

	orderedSlices := append([]string(nil), plan.EnabledSlices...)
	sort.Slice(orderedSlices, func(i, j int) bool {
		pi := slicePriorities[orderedSlices[i]]
		pj := slicePriorities[orderedSlices[j]]
		if pi != pj {
			return pi < pj // low to high for dropping
		}
		return orderedSlices[i] < orderedSlices[j]
	})

	keep := map[string]bool{}
	for _, s := range plan.EnabledSlices {
		keep[s] = true
	}

	for _, dropSlice := range orderedSlices {
		if err := ctx.Err(); err != nil {
			return Plan{}, "", err
		}
		// Never drop the highest remaining slice if it's the last one; break to tightening.
		if countKeptSlices(keep) <= 1 {
			break
		}
		keep[dropSlice] = false
		plan2.DroppedSlices = append(plan2.DroppedSlices, dropSlice)

		plan2.Included = filterIncludedByKept(plan.Included, keep)
		plan2.Dropped = append(plan2.Dropped, droppedFromRemovedSlices(plan.Included, keep)...)
		orderPlan(&plan2)

		r2, err := renderFn(plan2)
		if err != nil {
			return Plan{}, "", err
		}
		if runeCount(r2) <= b.Limits.MaxChars {
			return plan2, r2, nil
		}
	}

	// Tighten per-file truncation (halve max lines) once and retry.
	tight := plan2
	tight.Included = nil
	newMaxLines := max(1, b.Limits.PerFileMaxLines/2)
	for _, f := range plan2.Included {
		if err := ctx.Err(); err != nil {
			return Plan{}, "", err
		}
		entry, err := readAndTruncateFile(f.RelPath, f.AbsPath, f.Slices, f.PrimarySlice, f.Priority, newMaxLines, b.Limits.PerFileMaxBytes)
		if err != nil {
			// Treat any issue as unreadable/invalid and drop (partial).
			tight.Dropped = append(tight.Dropped, DroppedEntry{
				RelPath:      f.RelPath,
				Slices:       append([]string(nil), f.Slices...),
				PrimarySlice: f.PrimarySlice,
				Reason:       "unreadable",
				Detail:       err.Error(),
			})
			tight.Partial = true
			continue
		}
		tight.Included = append(tight.Included, entry)
	}
	orderPlan(&tight)
	r3, err := renderFn(tight)
	if err != nil {
		return Plan{}, "", err
	}
	if runeCount(r3) <= b.Limits.MaxChars {
		return tight, r3, nil
	}

	// Hard cut the rendered output.
	hard := tight
	hard.HardCut = true
	hard.Partial = true
	marker := "\n… [BUNDLE TRUNCATED: budget_exceeded]\n"
	hardCut := hardCutRunes(r3, b.Limits.MaxChars-len([]rune(marker)))
	if hardCut == "" {
		hardCut = marker
	} else {
		hardCut += marker
	}
	return hard, hardCut, nil
}

func filterIncludedByKept(in []FileEntry, keep map[string]bool) []FileEntry {
	var out []FileEntry
	for _, f := range in {
		if keep[f.PrimarySlice] {
			out = append(out, f)
		}
	}
	return out
}

func droppedFromRemovedSlices(in []FileEntry, keep map[string]bool) []DroppedEntry {
	var out []DroppedEntry
	for _, f := range in {
		if keep[f.PrimarySlice] {
			continue
		}
		out = append(out, DroppedEntry{
			RelPath:      f.RelPath,
			Slices:       append([]string(nil), f.Slices...),
			PrimarySlice: f.PrimarySlice,
			Reason:       "budget_exceeded",
			Detail:       "slice dropped",
		})
	}
	return out
}

func countKeptSlices(keep map[string]bool) int {
	n := 0
	for _, v := range keep {
		if v {
			n++
		}
	}
	return n
}

func runeCount(s string) int { return utf8.RuneCountInString(s) }

func hardCutRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}

func orderPlan(p *Plan) {
	sort.Slice(p.Included, func(i, j int) bool {
		if p.Included[i].Priority != p.Included[j].Priority {
			return p.Included[i].Priority > p.Included[j].Priority
		}
		return p.Included[i].RelPath < p.Included[j].RelPath
	})
	sort.Slice(p.Dropped, func(i, j int) bool { return p.Dropped[i].RelPath < p.Dropped[j].RelPath })
}

func readAndTruncateFile(rel, abs string, slices []string, primary string, priority int, maxLines, maxBytes int) (FileEntry, error) {
	st, err := os.Stat(abs)
	if err != nil {
		return FileEntry{}, err
	}
	origBytes := st.Size()

	f, err := os.Open(abs)
	if err != nil {
		return FileEntry{}, err
	}
	defer func() { _ = f.Close() }()

	var kept bytes.Buffer
	reader := bufio.NewReaderSize(f, 64*1024)

	origLines := 0
	keptLines := 0
	truncated := false

	var (
		seenAny          bool
		lastByteWasNL    bool
		lineBuf          bytes.Buffer
		keptByteCount    int
		byteBudget       = maxBytes
		lineBudget       = maxLines
		utf8ValidatorBuf []byte
		countOnly        bool // once true, we stop buffering line content (prevents huge final-line growth)
	)

	flushLine := func(force bool) {
		if lineBuf.Len() == 0 && !force {
			return
		}
		origLines++
		if keptLines < lineBudget && keptByteCount+lineBuf.Len() <= byteBudget {
			kept.Write(lineBuf.Bytes())
			keptLines++
			keptByteCount += lineBuf.Len()
		} else {
			truncated = true
		}
		lineBuf.Reset()

		// After flushing a line, if we can no longer keep anything else, switch to count-only mode.
		if keptLines >= lineBudget || keptByteCount >= byteBudget {
			if truncated {
				countOnly = true
			}
		}
	}

	for {
		b, err := reader.ReadByte()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return FileEntry{}, err
		}
		seenAny = true
		lastByteWasNL = b == '\n'

		utf8ValidatorBuf = append(utf8ValidatorBuf, b)
		ok, tail := util.FeedUTF8(utf8ValidatorBuf)
		if !ok {
			return FileEntry{}, errInvalidUTF8
		}
		utf8ValidatorBuf = tail

		if countOnly {
			// No buffering: just count lines.
			if b == '\n' {
				origLines++
			}
			continue
		}

		// Normal buffering path.
		lineBuf.WriteByte(b)

		// If budgets are already exhausted mid-line, do not let lineBuf grow without bound.
		// Flip to countOnly immediately and drop buffered bytes.
		if keptLines >= lineBudget || keptByteCount >= byteBudget {
			truncated = true
			countOnly = true
			lineBuf.Reset()
			if b == '\n' {
				origLines++ // newline closes a line even in countOnly mode
			}
			continue
		}

		if b == '\n' {
			flushLine(true)
		}
	}

	if len(utf8ValidatorBuf) != 0 {
		return FileEntry{}, errInvalidUTF8
	}

	// Count final line if file doesn't end with newline.
	if seenAny && !lastByteWasNL {
		if countOnly {
			origLines++
		} else {
			flushLine(true)
		}
	}

	content := kept.String()
	content = util.NormalizeNewlines(content)

	if truncated {
		marker := fmt.Sprintf("… [TRUNCATED: original_lines=%d kept_lines=%d]\n", origLines, keptLines)
		content += marker
	}

	return FileEntry{
		RelPath:       rel,
		AbsPath:       abs,
		Slices:        append([]string(nil), slices...),
		PrimarySlice:  primary,
		Priority:      priority,
		OriginalLines: origLines,
		OriginalBytes: origBytes,
		KeptLines:     keptLines,
		KeptBytes:     kept.Len(),
		Truncated:     truncated,
		Content:       content,
	}, nil
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
