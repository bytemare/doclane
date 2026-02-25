package syncer

import (
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"strings"
)

// checkAllowlist validates the source repo against optional owner/repo glob patterns.
func checkAllowlist(repo string, allowlist []string) error {
	if len(allowlist) == 0 {
		return nil
	}
	for _, pat := range allowlist {
		ok, err := path.Match(pat, repo)
		if err != nil {
			return fmt.Errorf("invalid source repo allowlist pattern %q: %w", pat, err)
		}
		if ok {
			return nil
		}
	}
	return fmt.Errorf("source repo %q is not in allowlist", repo)
}

// safeTargetPath ensures Doclane only writes within the consumer repository root.
func safeTargetPath(workdir, target string) (string, error) {
	if filepath.IsAbs(target) {
		return "", errors.New("target path must be relative")
	}
	clean := filepath.Clean(target)
	if clean == "." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || clean == ".." {
		return "", errors.New("target path escapes repository root")
	}
	joined := filepath.Join(workdir, clean)
	rel, err := filepath.Rel(workdir, joined)
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
		return "", errors.New("target path escapes repository root")
	}
	return joined, nil
}
