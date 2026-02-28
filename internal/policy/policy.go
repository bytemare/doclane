// SPDX-License-Identifier: MIT
//
// Copyright (C) 2026 Daniel Bourdrez. All Rights Reserved.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree or at
// https://spdx.org/licenses/MIT.html

package policy

import (
	"bytes"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/bytemare/doclane/internal/config"
	"github.com/bytemare/doclane/internal/selector"
	"gopkg.in/yaml.v3"
)

// Policy defines centrally managed Doclane governance rules.
type Policy struct {
	Version            int                `yaml:"version"`
	Required           []RequiredRule     `yaml:"required"`
	Constraints        Constraints        `yaml:"constraints"`
	SourceRequirements SourceRequirements `yaml:"source_requirements"`
}

// RequiredRule declares a mandatory sync entry.
type RequiredRule struct {
	ID     string `yaml:"id"`
	Source string `yaml:"source"`
	Target string `yaml:"target"`
}

// Constraints applies consumer-side sync constraints.
type Constraints struct {
	AllowedTargetPrefixes []string `yaml:"allowed_target_prefixes"`
	RequireDCO            bool     `yaml:"require_dco"`
	RequireCommitSigning  bool     `yaml:"require_commit_signing"`
	TemplateMode          string   `yaml:"template_mode"`
}

// SourceRequirements constrains the selectors and source trust mode consumers may use.
type SourceRequirements struct {
	AllowSelectors    []string `yaml:"allow_selectors"`
	RequireSignedTags bool     `yaml:"require_signed_tags"`
}

// EnforceOptions describes runtime commit behavior used during policy checks.
type EnforceOptions struct {
	PolicyMode   string
	CommitDCO    bool
	CommitSigned bool
}

// Parse unmarshals and validates a policy file.
func Parse(data []byte) (*Policy, error) {
	var p Policy
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&p); err != nil {
		return nil, err
	}
	if p.Version == 0 {
		p.Version = 1
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	return &p, nil
}

// Validate validates a parsed policy.
func (p *Policy) Validate() error {
	if p.Version != 1 {
		return fmt.Errorf("unsupported policy version %d", p.Version)
	}
	switch p.Constraints.TemplateMode {
	case "", "simple_placeholders_only":
	default:
		return fmt.Errorf("unsupported constraints.template_mode %q", p.Constraints.TemplateMode)
	}
	for i := range p.Required {
		r := p.Required[i]
		if r.ID == "" && (r.Source == "" || r.Target == "") {
			return fmt.Errorf("required[%d] must set id or both source+target", i)
		}
		if r.Target != "" {
			cleanTarget, err := config.NormalizeRepoRelativePath(r.Target)
			if err != nil {
				return fmt.Errorf("required[%d].target: %w", i, err)
			}
			p.Required[i].Target = cleanTarget
		}
	}
	for i := range p.Constraints.AllowedTargetPrefixes {
		prefix := p.Constraints.AllowedTargetPrefixes[i]
		if prefix == "" {
			return errors.New("constraints.allowed_target_prefixes cannot contain empty values")
		}
		cleanPrefix, err := config.NormalizeRepoRelativePath(prefix)
		if err != nil {
			return fmt.Errorf("constraints.allowed_target_prefixes[%d]: %w", i, err)
		}
		p.Constraints.AllowedTargetPrefixes[i] = cleanPrefix
	}
	return nil
}

// Enforce validates a consumer config against the supplied policy.
func Enforce(cfg *config.Config, sel selector.Selector, p *Policy, opts EnforceOptions) ([]string, error) {
	if p == nil {
		return nil, nil
	}
	var warnings []string
	addViolation := func(msg string) error {
		if cfg.Policy.Mode == "warn" || opts.PolicyMode == "warn" {
			warnings = append(warnings, msg)
			return nil
		}
		return errors.New(msg)
	}

	if p.Constraints.TemplateMode == "simple_placeholders_only" {
		for _, entry := range cfg.Sync {
			if entry.Template == nil {
				continue
			}
			for k := range entry.Template.Vars {
				if strings.TrimSpace(k) == "" {
					if err := addViolation(fmt.Sprintf("sync id %q contains an empty template var key", entry.ID)); err != nil {
						return warnings, err
					}
				}
			}
		}
	}

	if len(p.Constraints.AllowedTargetPrefixes) > 0 {
		for _, entry := range cfg.Sync {
			cleanTarget, err := config.NormalizeRepoRelativePath(entry.Target)
			if err != nil {
				if err := addViolation(fmt.Sprintf("sync target %q is invalid: %v", entry.Target, err)); err != nil {
					return warnings, err
				}
				continue
			}
			ok := false
			for _, rawPrefix := range p.Constraints.AllowedTargetPrefixes {
				prefix, err := config.NormalizeRepoRelativePath(rawPrefix)
				if err != nil {
					if err := addViolation(fmt.Sprintf("policy target prefix %q is invalid: %v", rawPrefix, err)); err != nil {
						return warnings, err
					}
					continue
				}
				if hasPathPrefix(cleanTarget, prefix) {
					ok = true
					break
				}
			}
			if !ok {
				if err := addViolation(fmt.Sprintf("sync target %q is not allowed by policy prefixes", cleanTarget)); err != nil {
					return warnings, err
				}
			}
		}
	}

	if len(p.SourceRequirements.AllowSelectors) > 0 {
		allowed := false
		for _, pat := range p.SourceRequirements.AllowSelectors {
			if selector.MatchesPolicyPattern(sel, pat) {
				allowed = true
				break
			}
		}
		if !allowed {
			if err := addViolation(fmt.Sprintf("selector %q is not allowed by policy", sel.String())); err != nil {
				return warnings, err
			}
		}
	}

	if p.Constraints.RequireDCO && !opts.CommitDCO {
		if err := addViolation("policy requires DCO sign-off for commits"); err != nil {
			return warnings, err
		}
	}
	if p.Constraints.RequireCommitSigning && !opts.CommitSigned {
		if err := addViolation("policy requires cryptographically signed commits"); err != nil {
			return warnings, err
		}
	}

	for _, req := range p.Required {
		if hasRequired(cfg.Sync, req) {
			continue
		}
		id := req.ID
		if id == "" {
			id = fmt.Sprintf("%s -> %s", req.Source, req.Target)
		}
		if err := addViolation(fmt.Sprintf("missing required sync entry %q", id)); err != nil {
			return warnings, err
		}
	}

	return warnings, nil
}

func hasPathPrefix(target, prefix string) bool {
	if target == prefix {
		return true
	}
	return strings.HasPrefix(target, prefix+string(filepath.Separator))
}

func hasRequired(entries []config.SyncEntry, req RequiredRule) bool {
	for _, e := range entries {
		if req.ID != "" {
			if e.ID == req.ID {
				if req.Source != "" && req.Source != e.Source {
					continue
				}
				if req.Target != "" && req.Target != e.Target {
					continue
				}
				return true
			}
			continue
		}
		if e.Source == req.Source && e.Target == req.Target {
			return true
		}
	}
	return false
}
