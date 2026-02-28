// SPDX-License-Identifier: MIT
//
// Copyright (C) 2026 Daniel Bourdrez. All Rights Reserved.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree or at
// https://spdx.org/licenses/MIT.html

package lockfile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Lockfile is the machine-managed Doclane state file.
type Lockfile struct {
	Version  int          `yaml:"version"`
	Resolved Resolved     `yaml:"resolved"`
	Policy   *PolicyState `yaml:"policy,omitempty"`
	Files    []FileState  `yaml:"files"`
}

// Resolved stores the immutable source version used for a sync run.
type Resolved struct {
	SourceRepo        string `yaml:"source_repo"`
	Selector          string `yaml:"selector"`
	ResolvedSelector  string `yaml:"resolved_selector,omitempty"`
	CommitSHA         string `yaml:"commit_sha"`
	CommitDate        string `yaml:"commit_date,omitempty"`
	TagVerified       bool   `yaml:"tag_verified,omitempty"`
	TagVerificationBy string `yaml:"tag_verification_by,omitempty"`
}

// PolicyState records the policy file provenance used for enforcement.
type PolicyState struct {
	Path        string `yaml:"path"`
	UpstreamSHA string `yaml:"upstream_sha256"`
	Enforced    bool   `yaml:"enforced"`
	Present     bool   `yaml:"present"`
}

// FileState records upstream and rendered hashes for one synced file.
type FileState struct {
	ID          string `yaml:"id"`
	Source      string `yaml:"source"`
	Target      string `yaml:"target"`
	UpstreamSHA string `yaml:"upstream_sha256"`
	RenderedSHA string `yaml:"rendered_sha256"`
	Templated   bool   `yaml:"templated"`
}

// LoadOptional loads a lockfile if it exists and returns nil values when absent.
func LoadOptional(path string) (*Lockfile, []byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	var lf Lockfile
	if err := yaml.Unmarshal(data, &lf); err != nil {
		return nil, nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &lf, data, nil
}

// Marshal serializes a lockfile to YAML.
func Marshal(lf *Lockfile) ([]byte, error) {
	if lf == nil {
		return nil, errors.New("nil lockfile")
	}
	if lf.Version == 0 {
		lf.Version = 1
	}
	out, err := yaml.Marshal(lf)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Write writes the lockfile to disk, creating parent directories as needed.
func Write(path string, lf *Lockfile) error {
	data, err := Marshal(lf)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
