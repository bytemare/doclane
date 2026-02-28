// SPDX-License-Identifier: MIT
//
// Copyright (C) 2026 Daniel Bourdrez. All Rights Reserved.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree or at
// https://spdx.org/licenses/MIT.html

package gitops

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// Runner executes git commands in a repository working directory.
type Runner struct {
	Dir string
}

// EnsureRepo verifies the working directory is a git work tree.
func (r Runner) EnsureRepo() error {
	_, err := r.run("rev-parse", "--is-inside-work-tree")
	return err
}

// CurrentBranch returns the current branch name.
func (r Runner) CurrentBranch() (string, error) {
	out, err := r.run("rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// SetUser configures git author identity in the local repository.
func (r Runner) SetUser(name, email string) error {
	if err := r.runQuiet("config", "user.name", name); err != nil {
		return err
	}
	return r.runQuiet("config", "user.email", email)
}

// CheckoutBranch creates or resets a local branch to HEAD.
func (r Runner) CheckoutBranch(branch string) error {
	return r.runQuiet("checkout", "-B", branch)
}

// Add stages the provided paths.
func (r Runner) Add(paths ...string) error {
	args := append([]string{"add", "--"}, paths...)
	return r.runQuiet(args...)
}

// Commit creates a commit with optional DCO sign-off and cryptographic signature.
func (r Runner) Commit(message string, signoff, sign bool) error {
	args := []string{"commit", "-m", message}
	if signoff {
		args = append(args, "--signoff")
	}
	if sign {
		args = append(args, "-S")
	}
	return r.runQuiet(args...)
}

// PushSetUpstream pushes a branch to origin with force-with-lease semantics.
func (r Runner) PushSetUpstream(branch string) error {
	return r.runQuiet("push", "--set-upstream", "origin", branch, "--force-with-lease")
}

// OriginRepo returns the origin remote parsed as owner/repo.
func (r Runner) OriginRepo() (string, error) {
	out, err := r.run("remote", "get-url", "origin")
	if err != nil {
		return "", err
	}
	return parseGitHubRepo(strings.TrimSpace(out))
}

func parseGitHubRepo(remote string) (string, error) {
	remote = strings.TrimSpace(remote)
	remote = strings.TrimSuffix(remote, ".git")
	switch {
	case strings.HasPrefix(remote, "https://github.com/"):
		return strings.TrimPrefix(remote, "https://github.com/"), nil
	case strings.HasPrefix(remote, "git@github.com:"):
		return strings.TrimPrefix(remote, "git@github.com:"), nil
	default:
		return "", fmt.Errorf("unsupported origin URL for GitHub repo detection: %q", remote)
	}
}

func (r Runner) runQuiet(args ...string) error {
	_, err := r.run(args...)
	return err
}

func (r Runner) run(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = filepath.Clean(r.Dir)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			msg := strings.TrimSpace(stderr.String())
			if msg == "" {
				msg = strings.TrimSpace(stdout.String())
			}
			return stdout.String(), fmt.Errorf("git %s failed: %s", strings.Join(args, " "), msg)
		}
		return stdout.String(), err
	}
	return stdout.String(), nil
}
