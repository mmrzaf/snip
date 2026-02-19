package gitinfo

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// ShortSHA returns the short git SHA for the repo at root.
// If git is unavailable or root is not a git repo, it returns "" and a non-nil error.
func ShortSHA(ctx context.Context, root string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--short", "HEAD")
	cmd.Dir = root
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git rev-parse: %w", err)
	}
	sha := strings.TrimSpace(out.String())
	return sha, nil
}
