package render

import (
	"bytes"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/mmrzaf/snip/internal/budget"
	"github.com/mmrzaf/snip/internal/util"
)

// BundleInfo is metadata for the bundle header.
type BundleInfo struct {
	Repo        string
	Root        string
	Profile     string
	Enabled     []string
	GitSHA      string
	Timestamp   time.Time
	SnipVersion string
}

// FileBlockOptions configures per-file delimiter markers.
type FileBlockOptions struct {
	Header string
	Footer string
}

// Renderer renders bundles.
type Renderer struct {
	Newline         string
	CodeFences      bool
	IncludeTree     bool
	TreeDepth       int
	IncludeManifest bool
	Manifest        ManifestOptions
	FileBlock       FileBlockOptions
}

// ManifestOptions controls manifest rendering.
type ManifestOptions struct {
	GroupBySlice           bool
	IncludeLineCounts      bool
	IncludeByteCounts      bool
	IncludeTruncationNotes bool
	IncludeUnreadableNotes bool
}

// RenderMarkdown renders plan as a markdown bundle.
func (r Renderer) RenderMarkdown(info BundleInfo, plan budget.Plan) (string, error) {
	nl := r.Newline
	if nl == "" {
		nl = "\n"
	}

	files := orderIncluded(plan.Included, r.Manifest.GroupBySlice)

	var buf bytes.Buffer
	write := func(s string) { buf.WriteString(s); buf.WriteString(nl) }

	write("# snip bundle")
	write("")
	write(fmt.Sprintf("repo: %s", info.Repo))
	write(fmt.Sprintf("root: %s", info.Root))
	write(fmt.Sprintf("profile: %s", info.Profile))
	write(fmt.Sprintf("enabled_slices: [%s]", strings.Join(info.Enabled, ", ")))
	write(fmt.Sprintf("git_sha: %s", info.GitSHA))
	write(fmt.Sprintf("timestamp: %s", info.Timestamp.Format(time.RFC3339)))
	write(fmt.Sprintf("snip_version: %s", info.SnipVersion))

	if r.IncludeTree {
		write("")
		write("## Tree")
		write("")
		buf.WriteString("```")
		buf.WriteString(nl)
		for _, line := range buildTree(files, r.TreeDepth) {
			buf.WriteString(line)
			buf.WriteString(nl)
		}
		buf.WriteString("```")
		buf.WriteString(nl)
	}

	if r.IncludeManifest {
		write("")
		write("## Manifest (included)")
		write("")
		buf.WriteString(renderManifestIncluded(files, r.Manifest, r.FileBlock, nl))
		write("")
		write("## Manifest (dropped)")
		write("")
		buf.WriteString(renderManifestDropped(plan.Dropped, plan.DroppedSlices, nl))
	}

	// Content.
	customDelims := r.FileBlock.Header != "" || r.FileBlock.Footer != ""
	for i, f := range files {
		idx := i + 1
		write("")

		if customDelims {
			h := applyFileBlockToken(r.FileBlock.Header, f.RelPath)
			if h != "" {
				write(h)
			}
			write(fmt.Sprintf("lines: %d", f.OriginalLines))
			write(fmt.Sprintf("bytes: %d", f.OriginalBytes))
			write(fmt.Sprintf("slices: [%s]", strings.Join(f.Slices, ", ")))
			write(fmt.Sprintf("truncated: %t", f.Truncated))
			write("")
		} else {
			write("---")
			write("")
			write(fmt.Sprintf("## %d) %s", idx, f.RelPath))
			write(fmt.Sprintf("lines: %d", f.OriginalLines))
			write(fmt.Sprintf("bytes: %d", f.OriginalBytes))
			write(fmt.Sprintf("slices: [%s]", strings.Join(f.Slices, ", ")))
			write(fmt.Sprintf("truncated: %t", f.Truncated))
			write("")
		}

		if r.CodeFences {
			lang := util.LanguageFromPath(f.RelPath)
			if lang != "" {
				buf.WriteString("```" + lang)
			} else {
				buf.WriteString("```")
			}
			buf.WriteString(nl)
			content := strings.ReplaceAll(f.Content, "\n", nl)
			buf.WriteString(content)
			if !strings.HasSuffix(content, nl) {
				buf.WriteString(nl)
			}
			buf.WriteString("```")
			buf.WriteString(nl)
		} else {
			content := strings.ReplaceAll(f.Content, "\n", nl)
			buf.WriteString(content)
			if !strings.HasSuffix(content, nl) {
				buf.WriteString(nl)
			}
		}

		if customDelims {
			foot := applyFileBlockToken(r.FileBlock.Footer, f.RelPath)
			if foot != "" {
				write(foot)
			}
		}
	}

	return buf.String(), nil
}

func applyFileBlockToken(s string, path string) string {
	if s == "" {
		return ""
	}
	return strings.ReplaceAll(s, "{path}", path)
}

func orderIncluded(files []budget.FileEntry, groupBySlice bool) []budget.FileEntry {
	out := append([]budget.FileEntry(nil), files...)
	if !groupBySlice {
		sort.Slice(out, func(i, j int) bool { return out[i].RelPath < out[j].RelPath })
		return out
	}
	// Group by primary slice; sort slice groups by priority desc then name.
	groups := map[string][]budget.FileEntry{}
	slicePri := map[string]int{}
	for _, f := range out {
		groups[f.PrimarySlice] = append(groups[f.PrimarySlice], f)
		if p, ok := slicePri[f.PrimarySlice]; !ok || f.Priority > p {
			slicePri[f.PrimarySlice] = f.Priority
		}
	}
	slices := make([]string, 0, len(groups))
	for s := range groups {
		slices = append(slices, s)
	}
	sort.Slice(slices, func(i, j int) bool {
		pi := slicePri[slices[i]]
		pj := slicePri[slices[j]]
		if pi != pj {
			return pi > pj
		}
		return slices[i] < slices[j]
	})

	ordered := make([]budget.FileEntry, 0, len(out))
	for _, s := range slices {
		g := groups[s]
		sort.Slice(g, func(i, j int) bool { return g[i].RelPath < g[j].RelPath })
		ordered = append(ordered, g...)
	}
	return ordered
}

func renderManifestIncluded(files []budget.FileEntry, opt ManifestOptions, fb FileBlockOptions, nl string) string {
	var buf bytes.Buffer

	// Self-describe delimiters for downstream parsers.
	if fb.Header != "" || fb.Footer != "" {
		if fb.Header != "" {
			buf.WriteString(fmt.Sprintf("delimiter_header: %q\n", fb.Header))
		}
		if fb.Footer != "" {
			buf.WriteString(fmt.Sprintf("delimiter_footer: %q\n", fb.Footer))
		}
		buf.WriteString("\n")
	}

	tw := tabwriter.NewWriter(&buf, 0, 4, 2, ' ', 0)

	idx := 1
	currentSlice := ""
	if opt.GroupBySlice {
		for _, f := range files {
			if f.PrimarySlice != currentSlice {
				currentSlice = f.PrimarySlice
				fmt.Fprintf(tw, "\n[%s]\n", currentSlice)
			}
			writeManifestLine(tw, idx, f, opt)
			idx++
		}
	} else {
		for _, f := range files {
			writeManifestLine(tw, idx, f, opt)
			idx++
		}
	}
	_ = tw.Flush()
	out := buf.String()
	out = strings.ReplaceAll(out, "\n", nl)
	if !strings.HasSuffix(out, nl) {
		out += nl
	}
	return out
}

func writeManifestLine(w *tabwriter.Writer, idx int, f budget.FileEntry, opt ManifestOptions) {
	parts := []string{}
	if opt.IncludeLineCounts {
		parts = append(parts, fmt.Sprintf("lines=%d", f.OriginalLines))
	}
	if opt.IncludeByteCounts {
		parts = append(parts, fmt.Sprintf("bytes=%d", f.OriginalBytes))
	}
	parts = append(parts, fmt.Sprintf("slices=[%s]", strings.Join(f.Slices, ",")))
	if opt.IncludeTruncationNotes {
		parts = append(parts, fmt.Sprintf("truncated=%t", f.Truncated))
	}
	fmt.Fprintf(w, "%3d\t%s\t%s\n", idx, f.RelPath, strings.Join(parts, " "))
}

func renderManifestDropped(dropped []budget.DroppedEntry, droppedSlices []string, nl string) string {
	var buf bytes.Buffer
	if len(droppedSlices) > 0 {
		slices := append([]string(nil), droppedSlices...)
		sort.Strings(slices)
		for _, s := range slices {
			buf.WriteString(fmt.Sprintf("- slice=%s reason=budget_exceeded", s))
			buf.WriteString("\n")
		}
	}
	for _, d := range dropped {
		note := fmt.Sprintf("- %s reason=%s", d.RelPath, d.Reason)
		if d.Detail != "" {
			note += " detail=" + sanitizeDetail(d.Detail)
		}
		if d.PrimarySlice != "" {
			note += " slice=" + d.PrimarySlice
		}
		buf.WriteString(note)
		buf.WriteString("\n")
	}
	out := strings.ReplaceAll(buf.String(), "\n", nl)
	if !strings.HasSuffix(out, nl) {
		out += nl
	}
	return out
}

func sanitizeDetail(s string) string {
	s = strings.ReplaceAll(s, "\t", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}

func buildTree(files []budget.FileEntry, depth int) []string {
	if depth <= 0 {
		depth = 1
	}
	paths := make([]string, 0, len(files))
	for _, f := range files {
		paths = append(paths, f.RelPath)
	}
	sort.Strings(paths)

	tree := newTreeNode(".")
	for _, p := range paths {
		parts := strings.Split(filepath.ToSlash(p), "/")
		tree.add(parts)
	}
	var out []string
	tree.render(&out, "", true, depth, 0)
	return out
}

type treeNode struct {
	name     string
	children map[string]*treeNode
	isFile   bool
}

func newTreeNode(name string) *treeNode {
	return &treeNode{name: name, children: map[string]*treeNode{}}
}

func (n *treeNode) add(parts []string) {
	cur := n
	for i, p := range parts {
		child, ok := cur.children[p]
		if !ok {
			child = newTreeNode(p)
			cur.children[p] = child
		}
		if i == len(parts)-1 {
			child.isFile = true
		}
		cur = child
	}
}

func (n *treeNode) render(out *[]string, prefix string, isLast bool, maxDepth int, depth int) {
	if depth == 0 {
		*out = append(*out, n.name)
	} else {
		branch := "├── "
		nextPrefix := prefix + "│   "
		if isLast {
			branch = "└── "
			nextPrefix = prefix + "    "
		}
		*out = append(*out, prefix+branch+n.name)
		prefix = nextPrefix
	}
	if depth >= maxDepth {
		return
	}

	names := make([]string, 0, len(n.children))
	for name := range n.children {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		a := n.children[names[i]]
		b := n.children[names[j]]
		if a.isFile != b.isFile {
			return !a.isFile && b.isFile
		}
		return names[i] < names[j]
	})

	for i, name := range names {
		child := n.children[name]
		child.render(out, prefix, i == len(names)-1, maxDepth, depth+1)
	}
}
