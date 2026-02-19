package discovery

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverClassifiesFilesByRules(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	mustWrite := func(rel string, data []byte) {
		t.Helper()
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("MkdirAll(%s): %v", rel, err)
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatalf("WriteFile(%s): %v", rel, err)
		}
	}

	mustWrite(".gitignore", []byte("ignored.txt\n"))
	mustWrite("README.md", []byte("hello"))
	mustWrite("ignored.txt", []byte("ignored by gitignore"))
	mustWrite("app-secret.txt", []byte("sensitive"))
	mustWrite("node_modules/lib.js", []byte("console.log(1)"))
	mustWrite("image.png", []byte("PNG"))
	mustWrite("binary.dat", []byte{0x00, 0x01, 0x02, 0x03})

	eng, err := NewEngine(
		root,
		true,
		[]string{"node_modules/**"},
		[]string{"**/*secret*"},
		[]string{".png"},
	)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	got, err := eng.Discover()
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	byPath := map[string]PathInfo{}
	for _, pi := range got {
		byPath[pi.RelPath] = pi
	}

	if pi := byPath["README.md"]; pi.Excluded {
		t.Fatalf("README.md should be included: %+v", pi)
	}
	if pi := byPath["ignored.txt"]; pi.ExclusionReason != ExcludedGitignore {
		t.Fatalf("ignored.txt reason=%q", pi.ExclusionReason)
	}
	if pi := byPath["app-secret.txt"]; pi.ExclusionReason != ExcludedSensitive {
		t.Fatalf("app-secret.txt reason=%q", pi.ExclusionReason)
	}
	if pi := byPath["image.png"]; pi.ExclusionReason != ExcludedBinary {
		t.Fatalf("image.png reason=%q", pi.ExclusionReason)
	}
	if pi := byPath["binary.dat"]; pi.ExclusionReason != ExcludedBinary {
		t.Fatalf("binary.dat reason=%q", pi.ExclusionReason)
	}
}
