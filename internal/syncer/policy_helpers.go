// SPDX-License-Identifier: MIT
//
// Copyright (C) 2026 Daniel Bourdrez. All Rights Reserved.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree or at
// https://spdx.org/licenses/MIT.html

package syncer

import (
	"context"
	"errors"
	"fmt"

	"github.com/bytemare/doclane/internal/config"
	"github.com/bytemare/doclane/internal/githubapi"
	"github.com/bytemare/doclane/internal/hashutil"
	"github.com/bytemare/doclane/internal/lockfile"
	"github.com/bytemare/doclane/internal/policy"
	"github.com/bytemare/doclane/internal/selector"
)

// loadPolicy fetches and validates the central policy file at the resolved source commit.
func loadPolicy(ctx context.Context, client sourceFileFetcher, cfg *config.Config, resolved githubapi.ResolvedRef) (*lockfile.PolicyState, []string, *policy.Policy, error) {
	var warnings []string
	data, err := client.FetchFile(ctx, cfg.Source.Repo, cfg.Policy.Path, resolved.CommitSHA)
	if err != nil {
		if errors.Is(err, githubapi.ErrNotFound) {
			msg := fmt.Sprintf("policy file %q not found in %s@%s", cfg.Policy.Path, cfg.Source.Repo, resolved.CommitSHA)
			if cfg.Policy.Mode == "warn" {
				warnings = append(warnings, msg)
				return &lockfile.PolicyState{Path: cfg.Policy.Path, Present: false, Enforced: false}, warnings, nil, nil
			}
			return nil, warnings, nil, errors.New(msg)
		}
		return nil, warnings, nil, fmt.Errorf("fetch policy file: %w", err)
	}
	p, err := policy.Parse(data)
	if err != nil {
		if cfg.Policy.Mode == "warn" {
			warnings = append(warnings, fmt.Sprintf("invalid policy file %q: %v", cfg.Policy.Path, err))
			return &lockfile.PolicyState{
				Path:        cfg.Policy.Path,
				UpstreamSHA: hashutil.SHA256Hex(data),
				Present:     true,
				Enforced:    false,
			}, warnings, nil, nil
		}
		return nil, warnings, nil, fmt.Errorf("parse policy file: %w", err)
	}
	return &lockfile.PolicyState{
		Path:        cfg.Policy.Path,
		UpstreamSHA: hashutil.SHA256Hex(data),
		Present:     true,
		Enforced:    cfg.Policy.Mode == "enforce",
	}, warnings, p, nil
}

// requiresSignedTag returns true when the resolved selector must be a verified tag.
func requiresSignedTag(cfg *config.Config, pol *policy.Policy, sel selector.Selector) bool {
	if sel.Kind != selector.KindTag {
		return false
	}
	if cfg.Source.EnforceSignedTags {
		return true
	}
	if pol != nil && pol.SourceRequirements.RequireSignedTags {
		return true
	}
	return false
}
