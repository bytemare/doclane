// SPDX-License-Identifier: MIT
//
// Copyright (C) 2026 Daniel Bourdrez. All Rights Reserved.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree or at
// https://spdx.org/licenses/MIT.html

package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bytemare/doclane/internal/selector"
	"gopkg.in/yaml.v3"
)

const (
	// DefaultConfigPath is the default consumer configuration file location.
	DefaultConfigPath = ".github/.doclane.yml"
	// DefaultLockfilePath is the default consumer lockfile location.
	DefaultLockfilePath = ".github/.doclane.lock.yml"
	// DefaultPolicyPath is the default policy file path in the source repository.
	DefaultPolicyPath = ".doclane-policy.yml"
)

// Config is the user-managed Doclane configuration file.
type Config struct {
	Version int          `yaml:"version"`
	Source  SourceConfig `yaml:"source"`
	Policy  PolicyConfig `yaml:"policy"`
	Sync    []SyncEntry  `yaml:"sync"`
}

// SourceConfig defines where shared files are fetched from and how updates are tracked.
type SourceConfig struct {
	Repo              string `yaml:"repo"`
	Selector          string `yaml:"selector"`
	AllowLatest       bool   `yaml:"allow_latest"`
	EnforceSignedTags bool   `yaml:"enforce_signed_tags"`
}

// PolicyConfig controls central policy fetching and enforcement mode.
type PolicyConfig struct {
	Mode string `yaml:"mode"`
	Path string `yaml:"path"`
}

// SyncEntry maps one upstream file to one target file in the consumer repository.
type SyncEntry struct {
	ID       string        `yaml:"id"`
	Source   string        `yaml:"source"`
	Target   string        `yaml:"target"`
	Template *TemplateSpec `yaml:"template,omitempty"`
}

// TemplateSpec defines simple placeholder substitutions applied to a source file.
type TemplateSpec struct {
	Vars map[string]string `yaml:"vars"`
}

// Load reads, defaults, and validates a Doclane config file.
func Load(path string) (*Config, []byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	var cfg Config
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		return nil, nil, fmt.Errorf("parse %s: %w", path, err)
	}
	cfg.applyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, nil, err
	}
	return &cfg, data, nil
}

func (c *Config) applyDefaults() {
	if c.Version == 0 {
		c.Version = 1
	}
	if c.Policy.Mode == "" {
		c.Policy.Mode = "enforce"
	}
	if c.Policy.Path == "" {
		c.Policy.Path = DefaultPolicyPath
	}
}

// Validate validates a Doclane config after defaults are applied.
func (c *Config) Validate() error {
	if c.Version != 1 {
		return fmt.Errorf("unsupported config version %d", c.Version)
	}
	if c.Source.Repo == "" {
		return errors.New("source.repo is required")
	}
	if strings.Count(c.Source.Repo, "/") != 1 {
		return fmt.Errorf("source.repo must be in owner/repo form, got %q", c.Source.Repo)
	}
	if c.Source.Selector == "" {
		return errors.New("source.selector is required")
	}
	if _, err := selector.Parse(c.Source.Selector); err != nil {
		return fmt.Errorf("source.selector: %w", err)
	}
	if c.Source.Selector == "latest" && !c.Source.AllowLatest {
		return errors.New("source.selector=latest requires source.allow_latest=true")
	}
	switch c.Policy.Mode {
	case "enforce", "warn":
	default:
		return fmt.Errorf("policy.mode must be enforce or warn, got %q", c.Policy.Mode)
	}
	if c.Policy.Path == "" {
		return errors.New("policy.path cannot be empty")
	}
	cleanPolicyPath, err := NormalizeRepoRelativePath(c.Policy.Path)
	if err != nil {
		return fmt.Errorf("policy.path: %w", err)
	}
	c.Policy.Path = cleanPolicyPath
	if len(c.Sync) == 0 {
		return errors.New("sync must contain at least one entry")
	}

	targets := make(map[string]string, len(c.Sync))
	ids := make(map[string]struct{}, len(c.Sync))
	for i := range c.Sync {
		entry := c.Sync[i]
		if entry.ID == "" {
			return fmt.Errorf("sync[%d].id is required", i)
		}
		if _, ok := ids[entry.ID]; ok {
			return fmt.Errorf("duplicate sync id %q", entry.ID)
		}
		ids[entry.ID] = struct{}{}
		if strings.TrimSpace(entry.Source) == "" {
			return fmt.Errorf("sync[%d].source is required", i)
		}
		if strings.TrimSpace(entry.Target) == "" {
			return fmt.Errorf("sync[%d].target is required", i)
		}
		cleanTarget, err := NormalizeRepoRelativePath(entry.Target)
		if err != nil {
			return fmt.Errorf("sync[%d].target: %w", i, err)
		}
		c.Sync[i].Target = cleanTarget
		if prev, ok := targets[cleanTarget]; ok {
			return fmt.Errorf("duplicate sync target %q (ids %q and %q)", cleanTarget, prev, entry.ID)
		}
		targets[cleanTarget] = entry.ID
		if entry.Template != nil && entry.Template.Vars == nil {
			return fmt.Errorf("sync[%d].template.vars is required when template is set", i)
		}
	}
	return nil
}

// NormalizeRepoRelativePath validates and canonicalizes a repository-relative path.
func NormalizeRepoRelativePath(p string) (string, error) {
	if filepath.IsAbs(p) {
		return "", fmt.Errorf("must be repository-relative, got %q", p)
	}
	clean := filepath.Clean(p)
	if clean == "." {
		return "", errors.New("must not be current directory")
	}
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", errors.New("path escapes repository root")
	}
	return clean, nil
}
