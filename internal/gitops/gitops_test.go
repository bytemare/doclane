// SPDX-License-Identifier: MIT
//
// Copyright (C) 2026 Daniel Bourdrez. All Rights Reserved.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree or at
// https://spdx.org/licenses/MIT.html

package gitops

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func initRepo(t *testing.T, dir string) {
	t.Helper()

	cmd := exec.Command("git", "init", "-b", "main")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		_ = out
		runGit(t, dir, "init")
		runGit(t, dir, "checkout", "-b", "main")
	}
}

func TestParseGitHubRepo(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		remote  string
		want    string
		wantErr bool
	}{
		{name: "https", remote: "https://github.com/acme/shared.git", want: "acme/shared"},
		{name: "ssh", remote: "git@github.com:acme/shared.git", want: "acme/shared"},
		{name: "unsupported", remote: "ssh://example.com/acme/shared.git", wantErr: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseGitHubRepo(tt.remote)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseGitHubRepo returned error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("unexpected parsed repo: %q", got)
			}
		})
	}
}

func TestRunnerBasicLifecycle(t *testing.T) {
	t.Parallel()

	const testBranch = "chore/doclane-sync/test"
	dir := t.TempDir()
	r := Runner{Dir: dir}

	if err := r.EnsureRepo(); err == nil {
		t.Fatal("expected EnsureRepo to fail outside git repo")
	}

	initRepo(t, dir)

	if err := r.EnsureRepo(); err != nil {
		t.Fatalf("EnsureRepo returned error: %v", err)
	}
	if err := r.SetUser("Doclane Bot", "bot@example.com"); err != nil {
		t.Fatalf("SetUser returned error: %v", err)
	}
	runGit(t, dir, "config", "commit.gpgsign", "false")

	target := filepath.Join(dir, "docs", "README.md")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	if err := os.WriteFile(target, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	if err := r.Add("docs/README.md"); err != nil {
		t.Fatalf("Add returned error: %v", err)
	}
	if err := r.Commit("test commit", true, false); err != nil {
		t.Fatalf("Commit returned error: %v", err)
	}

	branch, err := r.CurrentBranch()
	if err != nil {
		t.Fatalf("CurrentBranch returned error: %v", err)
	}
	if branch == "" || branch == "HEAD" {
		t.Fatalf("unexpected branch: %q", branch)
	}

	if err := r.CheckoutBranch(testBranch); err != nil {
		t.Fatalf("CheckoutBranch returned error: %v", err)
	}
	branch, err = r.CurrentBranch()
	if err != nil {
		t.Fatalf("CurrentBranch returned error: %v", err)
	}
	if branch != testBranch {
		t.Fatalf("unexpected checked-out branch: %q", branch)
	}

	remote := filepath.Join(t.TempDir(), "origin.git")
	runGit(t, dir, "init", "--bare", remote)
	runGit(t, dir, "remote", "add", "origin", remote)
	if err := r.PushSetUpstream(testBranch); err != nil {
		t.Fatalf("PushSetUpstream returned error: %v", err)
	}
}

func TestOriginRepo(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	initRepo(t, dir)
	r := Runner{Dir: dir}

	runGit(t, dir, "remote", "add", "origin", "https://github.com/acme/consumer.git")
	got, err := r.OriginRepo()
	if err != nil {
		t.Fatalf("OriginRepo returned error: %v", err)
	}
	if got != "acme/consumer" {
		t.Fatalf("unexpected origin repo: %q", got)
	}

	runGit(t, dir, "remote", "set-url", "origin", "git@github.com:acme/consumer.git")
	got, err = r.OriginRepo()
	if err != nil {
		t.Fatalf("OriginRepo returned error: %v", err)
	}
	if got != "acme/consumer" {
		t.Fatalf("unexpected origin repo from ssh URL: %q", got)
	}

	runGit(t, dir, "remote", "set-url", "origin", "https://example.com/acme/consumer.git")
	if _, err := r.OriginRepo(); err == nil {
		t.Fatal("expected unsupported origin URL error")
	}
}

func TestRunWrapsGitExitErrors(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	initRepo(t, dir)
	r := Runner{Dir: dir}
	_, err := r.run("status", "--not-a-real-flag")
	if err == nil {
		t.Fatal("expected git command to fail")
	}
	if !strings.Contains(err.Error(), "git status --not-a-real-flag failed") {
		t.Fatalf("unexpected wrapped error: %v", err)
	}
}
