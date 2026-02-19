package util

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"unicode/utf8"
)

var reToken = regexp.MustCompile(`\{[A-Za-z0-9_]+\}`)

// ApplyPatternTokens substitutes tokens like {ts} with values.
// Unknown tokens are preserved.
func ApplyPatternTokens(pattern string, values map[string]string) string {
	out := reToken.ReplaceAllStringFunc(pattern, func(m string) string {
		k := strings.TrimSuffix(strings.TrimPrefix(m, "{"), "}")
		if v, ok := values[k]; ok {
			return v
		}
		return m
	})
	return CollapseSeparators(out)
}

// CollapseSeparators collapses repeated separators commonly produced by empty tokens.
func CollapseSeparators(s string) string {
	for strings.Contains(s, "__") {
		s = strings.ReplaceAll(s, "__", "_")
	}
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	for strings.Contains(s, "..") {
		s = strings.ReplaceAll(s, "..", ".")
	}
	s = strings.Trim(s, "_-.")
	return s
}

// NormalizeNewlines converts CRLF and CR to LF.
func NormalizeNewlines(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	if strings.Contains(s, "\r") {
		s = strings.ReplaceAll(s, "\r", "\n")
	}
	return s
}

// LanguageFromPath returns a best-effort code fence language.
func LanguageFromPath(p string) string {
	ext := strings.ToLower(filepath.Ext(p))
	switch ext {
	case ".go":
		return "go"
	case ".js":
		return "javascript"
	case ".ts":
		return "typescript"
	case ".py":
		return "python"
	case ".rb":
		return "ruby"
	case ".java":
		return "java"
	case ".kt":
		return "kotlin"
	case ".rs":
		return "rust"
	case ".c":
		return "c"
	case ".h":
		return "c"
	case ".cpp", ".cc", ".cxx", ".hpp":
		return "cpp"
	case ".cs":
		return "csharp"
	case ".php":
		return "php"
	case ".sh":
		return "bash"
	case ".sql":
		return "sql"
	case ".yaml", ".yml":
		return "yaml"
	case ".json":
		return "json"
	case ".md":
		return "md"
	default:
		return ""
	}
}

// SniffBinary returns true if the byte sample appears binary.
func SniffBinary(sample []byte) bool {
	if len(sample) == 0 {
		return false
	}
	nul := bytesContains(sample, 0)
	if nul {
		return true
	}
	// Count non-text bytes.
	var non int
	for _, b := range sample {
		// Allow common whitespace and printable ASCII.
		if b == '\n' || b == '\r' || b == '\t' {
			continue
		}
		if b >= 0x20 && b <= 0x7e {
			continue
		}
		non++
	}
	ratio := float64(non) / float64(len(sample))
	return ratio > 0.30
}

func bytesContains(b []byte, v byte) bool {
	for _, x := range b {
		if x == v {
			return true
		}
	}
	return false
}

// FeedUTF8 incrementally validates UTF-8.
// It returns (ok, tail) where tail is any incomplete rune prefix to carry over.
func FeedUTF8(buf []byte) (bool, []byte) {
	for len(buf) > 0 {
		r, size := utf8.DecodeRune(buf)
		if r == utf8.RuneError && size == 1 {
			// Could be invalid or incomplete. If incomplete, keep tail.
			if len(buf) < utf8.UTFMax && !utf8.FullRune(buf) {
				return true, buf
			}
			return false, nil
		}
		buf = buf[size:]
	}
	return true, nil
}

// AtomicWriteFile writes file content atomically by writing to a temp file and renaming.
func AtomicWriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	base := filepath.Base(path)
	tmp := filepath.Join(dir, fmt.Sprintf(".%s.tmp.%d", base, os.Getpid()))
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return fmt.Errorf("open temp: %w", err)
	}
	_, werr := f.Write(data)
	cerr := f.Close()
	if werr != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("write temp: %w", werr)
	}
	if cerr != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("close temp: %w", cerr)
	}
	if runtime.GOOS != "windows" {
		// Best-effort fsync on POSIX.
		df, err := os.Open(dir)
		if err == nil {
			_ = df.Sync()
			_ = df.Close()
		}
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}

// SecureRandomString returns a URL-safe random string of length n (approx).
func SecureRandomString(n int) (string, error) {
	if n <= 0 {
		return "", errors.New("n must be > 0")
	}
	b := make([]byte, n)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", err
	}
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	var sb strings.Builder
	sb.Grow(n)
	for _, x := range b {
		sb.WriteByte(alphabet[int(x)%len(alphabet)])
	}
	return sb.String(), nil
}

// NextCounter increments (and persists) an integer counter in the given directory.
//
// The counter is stored in a file named 'counter' inside dir, and is incremented on each call.
// The update uses AtomicWriteFile for best-effort atomicity on a single filesystem.
func NextCounter(dir string) (int, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, fmt.Errorf("mkdir: %w", err)
	}
	path := filepath.Join(dir, "counter")
	cur := 0
	if b, err := os.ReadFile(path); err == nil {
		text := strings.TrimSpace(string(b))
		if text != "" {
			v, perr := strconv.Atoi(text)
			if perr != nil {
				return 0, fmt.Errorf("parse counter: %w", perr)
			}
			cur = v
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return 0, fmt.Errorf("read counter: %w", err)
	}
	next := cur + 1
	if err := AtomicWriteFile(path, []byte(fmt.Sprintf("%d\n", next)), 0o644); err != nil {
		return 0, fmt.Errorf("write counter: %w", err)
	}
	return next, nil
}
