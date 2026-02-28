// SPDX-License-Identifier: MIT
//
// Copyright (C) 2026 Daniel Bourdrez. All Rights Reserved.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree or at
// https://spdx.org/licenses/MIT.html

package syncer

import (
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"github.com/bytemare/doclane/internal/config"
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
	joined, _, err := safeRepoRelativePath(workdir, target)
	return joined, err
}

// safeRepoRelativePath validates a repo-relative path and returns absolute + normalized forms.
func safeRepoRelativePath(workdir, relPath string) (string, string, error) {
	clean, err := config.NormalizeRepoRelativePath(relPath)
	if err != nil {
		return "", "", err
	}
	joined := filepath.Join(workdir, clean)
	rel, err := filepath.Rel(workdir, joined)
	if err != nil {
		return "", "", err
	}
	if strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
		return "", "", errors.New("path escapes repository root")
	}
	return joined, clean, nil
}
