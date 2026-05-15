package apply

import (
	"strings"

	"github.com/mmrzaf/snip/internal/util"
)

// Block is one parsed file payload.
type Block struct {
	Path    string // as declared in input (trimmed)
	Content []byte // exact bytes between opening and closing fence after newline normalization
}

// fenceInfo holds the parsed properties of an opening fence.
type fenceInfo struct {
	char  byte
	count int
}

// parseFenceOpen examines a line and returns fence info if it is a valid opening fence.
// It follows CommonMark: a line that begins with at least three backticks or tildes.
// The remainder of the line may contain an info string (e.g., language).
func parseFenceOpen(line string) (info fenceInfo, ok bool) {
	trimmed := strings.TrimSpace(line)
	if len(trimmed) < 3 {
		return fenceInfo{}, false
	}
	c := trimmed[0]
	if c != '`' && c != '~' {
		return fenceInfo{}, false
	}
	n := 0
	for n < len(trimmed) && trimmed[n] == c {
		n++
	}
	if n < 3 {
		return fenceInfo{}, false
	}
	return fenceInfo{char: c, count: n}, true
}

// isFenceClose checks if a line is a valid closing fence for the given opening info.
// It follows CommonMark: the line must start with at least minCount fence chars
// (after stripping leading whitespace), and the remainder (after all fence chars)
// must consist only of whitespace (i.e., no info string).
func isFenceClose(line string, open fenceInfo) bool {
	leftTrimmed := strings.TrimLeft(line, " \t")
	if len(leftTrimmed) < open.count {
		return false
	}
	// Must start with the same fence char repeated at least open.count times.
	for i := 0; i < open.count; i++ {
		if leftTrimmed[i] != open.char {
			return false
		}
	}
	// Any additional characters beyond the minimum count must also be the fence char.
	i := open.count
	for i < len(leftTrimmed) && leftTrimmed[i] == open.char {
		i++
	}
	// The rest of the line (after stripping all fence chars) must be only whitespace.
	rest := leftTrimmed[i:]
	return strings.TrimSpace(rest) == ""
}

// Parse extracts file blocks from markdown-like text using a header template.
// Header must contain exactly one {path} token. Handles nested fences correctly.
func Parse(input string, fileHeader string) ([]Block, error) {
	hm, err := compileHeaderMatcher(fileHeader)
	if err != nil {
		return nil, err
	}

	src := input
	var blocks []Block
	seen := make(map[string]int)

	// Track whether we are inside a *non-apply* fenced block.
	inFence := false
	var fenceInfoCurrent fenceInfo

	i := 0
	lineNo := 0
	for {
		line, next, ok := readLine(src, i)
		if !ok {
			break
		}
		i = next
		lineNo++

		if inFence {
			if isFenceClose(line, fenceInfoCurrent) {
				inFence = false
				fenceInfoCurrent = fenceInfo{}
			}
			continue
		}

		// If we see an arbitrary opening fence, enter fence mode.
		if info, ok := parseFenceOpen(line); ok {
			inFence = true
			fenceInfoCurrent = info
			continue
		}

		// Only now do we consider header matching.
		path, matched := hm.match(line)
		if !matched {
			continue
		}

		path = strings.TrimSpace(path)
		if path == "" {
			return nil, invalidf("empty path in header at line %d", lineNo)
		}
		if prev, dup := seen[path]; dup {
			return nil, invalidf("duplicate file path %q (headers at lines %d and %d)", path, prev, lineNo)
		}
		seen[path] = lineNo

		// Scan forward for the next opening fence (the outer fence of the file block).
		var (
			foundOpen    bool
			openLineNo   int
			blockFence   fenceInfo
			contentStart int
		)

		j := i
		ln := lineNo
		for {
			l2, next2, ok2 := readLine(src, j)
			if !ok2 {
				break
			}
			ln++

			// A second header before any fence is ambiguity => fail.
			if _, m2 := hm.match(l2); m2 {
				return nil, invalidf("header at line %d for %q has no code fence before next header at line %d", lineNo, path, ln)
			}

			if info, ok := parseFenceOpen(l2); ok {
				foundOpen = true
				openLineNo = ln
				blockFence = info
				contentStart = next2
				j = next2
				break
			}
			j = next2
		}
		if !foundOpen {
			return nil, invalidf("header at line %d for %q has no code fence", lineNo, path)
		}

		// Use a stack to track nested fences. Outer block ends when stack becomes empty.
		stack := []fenceInfo{blockFence}
		contentEnd := -1
		closeLineNo := 0

		for {
			l3, next3, ok3 := readLine(src, j)
			if !ok3 {
				break
			}
			ln++

			if len(stack) > 0 {
				top := stack[len(stack)-1]
				if isFenceClose(l3, top) {
					stack = stack[:len(stack)-1]
					if len(stack) == 0 {
						closeLineNo = ln
						contentEnd = j
						j = next3
						break
					}
					j = next3
					continue
				}
			}

			if openInfo, okOpen := parseFenceOpen(l3); okOpen {
				stack = append(stack, openInfo)
			}
			j = next3
		}

		if closeLineNo == 0 {
			return nil, invalidf("unclosed code fence for %q (header line %d, fence line %d)", path, lineNo, openLineNo)
		}

		content := src[contentStart:contentEnd]
		blocks = append(blocks, Block{
			Path:    path,
			Content: []byte(content),
		})

		i = j
		lineNo = closeLineNo
	}

	if len(blocks) == 0 {
		return nil, invalidf("no file blocks detected")
	}
	return blocks, nil
}

type headerMatcher struct {
	prefix string
	suffix string
}

func compileHeaderMatcher(tpl string) (headerMatcher, error) {
	tpl = util.NormalizeNewlines(tpl)
	tpl = strings.TrimSuffix(tpl, "\n")
	if strings.TrimSpace(tpl) == "" {
		return headerMatcher{}, invalidf("file header template is required (must contain {path})")
	}
	if strings.Count(tpl, "{path}") != 1 {
		return headerMatcher{}, invalidf("file header template must contain exactly one {path} token")
	}
	idx := strings.Index(tpl, "{path}")
	return headerMatcher{
		prefix: tpl[:idx],
		suffix: tpl[idx+len("{path}"):],
	}, nil
}

func (m headerMatcher) match(line string) (string, bool) {
	if !strings.HasPrefix(line, m.prefix) {
		return "", false
	}
	if !strings.HasSuffix(line, m.suffix) {
		return "", false
	}
	return line[len(m.prefix) : len(line)-len(m.suffix)], true
}

// readLine returns the next line (without newline) and the next position.
func readLine(s string, start int) (line string, next int, ok bool) {
	if start >= len(s) {
		return "", start, false
	}
	i := start
	for i < len(s) && s[i] != '\n' {
		i++
	}
	if i < len(s) {
		return s[start:i], i + 1, true
	}
	return s[start:i], i, true
}
