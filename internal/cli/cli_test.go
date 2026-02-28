// SPDX-License-Identifier: MIT
//
// Copyright (C) 2026 Daniel Bourdrez. All Rights Reserved.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree or at
// https://spdx.org/licenses/MIT.html

package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bytemare/doclane/internal/syncer"
)

func writeConfig(t *testing.T, dir string, data string) string {
	t.Helper()

	path := filepath.Join(dir, ".github", ".doclane.yml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir .github: %v", err)
	}
	if err := os.WriteFile(path, []byte(strings.TrimSpace(data)+"\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestRunRootUsageAndVersion(t *testing.T) {
	t.Parallel()

	if got := Run(nil); got != 2 {
		t.Fatalf("expected exit code 2 for empty args, got %d", got)
	}
	if got := Run([]string{"unknown"}); got != 2 {
		t.Fatalf("expected exit code 2 for unknown command, got %d", got)
	}
	if got := Run([]string{"version"}); got != 0 {
		t.Fatalf("expected exit code 0 for version, got %d", got)
	}
}

func TestRunValidate(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	validConfigPath := writeConfig(t, dir, `
version: 1
source:
  repo: acme/shared
  selector: commit:0123456789abcdef0123456789abcdef01234567
sync:
  - id: readme
    source: README.md
    target: docs/README.md
`)
	if got := Run([]string{"validate", "--config-path", validConfigPath}); got != 0 {
		t.Fatalf("expected validate success exit 0, got %d", got)
	}

	invalidConfigPath := writeConfig(t, dir, `
version: 1
source:
  repo: acme/shared
  selector: latest
sync:
  - id: readme
    source: README.md
    target: docs/README.md
`)
	if got := Run([]string{"validate", "--config-path", invalidConfigPath}); got != 1 {
		t.Fatalf("expected validate failure exit 1, got %d", got)
	}
}

func TestRunSyncInvalidFlags(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")
	if got := Run([]string{"sync"}); got != 2 {
		t.Fatalf("expected invalid flags exit 2, got %d", got)
	}
	if got := Run([]string{"sync", "--unknown-flag"}); got != 2 {
		t.Fatalf("expected parse error exit 2, got %d", got)
	}
}

func TestRunValidateFlagParseError(t *testing.T) {
	t.Parallel()

	if got := Run([]string{"validate", "--unknown-flag"}); got != 2 {
		t.Fatalf("expected validate parse error exit 2, got %d", got)
	}
}

func TestRunSyncReturnsFailureOnSyncError(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	configPath := writeConfig(t, dir, `
version: 1
source:
  repo: acme/shared
  selector: latest
sync:
  - id: readme
    source: README.md
    target: docs/README.md
`)
	if got := Run([]string{
		"sync",
		"--workdir", dir,
		"--config-path", configPath,
		"--lockfile-path", ".github/.doclane.lock.yml",
		"--manifest-path", "doclane-sync-manifest.json",
		"--source-repo-allowlist", "acme/*, bytemare/*",
		"--github-token", "token",
	}); got != 1 {
		t.Fatalf("expected sync failure exit 1, got %d", got)
	}
}

func TestWriteGitHubOutputs(t *testing.T) {
	t.Setenv("GITHUB_OUTPUT", "")
	writeGitHubOutputs(syncer.Result{Noop: true})

	dir := t.TempDir()
	t.Setenv("GITHUB_OUTPUT", dir)
	writeGitHubOutputs(syncer.Result{Noop: true})

	outputPath := filepath.Join(dir, "outputs", "gh.out")
	t.Setenv("GITHUB_OUTPUT", outputPath)

	writeGitHubOutputs(syncer.Result{
		Noop: false,
		Resolved: syncer.ResolvedResult{
			CommitSHA: "0123456789abcdef0123456789abcdef01234567",
		},
		PRURL: "https://github.com/acme/consumer/pull/42",
	})

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read output file: %v", err)
	}
	got := string(data)
	if !strings.Contains(got, "noop=false\n") {
		t.Fatalf("missing noop output: %q", got)
	}
	if !strings.Contains(got, "resolved-sha=0123456789abcdef0123456789abcdef01234567\n") {
		t.Fatalf("missing resolved-sha output: %q", got)
	}
	if !strings.Contains(got, "pr-url=https://github.com/acme/consumer/pull/42\n") {
		t.Fatalf("missing pr-url output: %q", got)
	}

	outputPathNoop := filepath.Join(dir, "outputs", "gh-noop.out")
	t.Setenv("GITHUB_OUTPUT", outputPathNoop)
	writeGitHubOutputs(syncer.Result{Noop: true})
	dataNoop, err := os.ReadFile(outputPathNoop)
	if err != nil {
		t.Fatalf("read noop output file: %v", err)
	}
	if gotNoop := string(dataNoop); gotNoop != "noop=true\n" {
		t.Fatalf("expected noop-only output, got %q", gotNoop)
	}
}
