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

	"github.com/bytemare/doclane/internal/githubapi"
	"github.com/bytemare/doclane/internal/selector"
)

// sourceFileFetcher is the minimal upstream file-fetch capability used by sync helpers.
type sourceFileFetcher interface {
	FetchFile(ctx context.Context, repo, filePath, ref string) ([]byte, error)
}

// sourceClient is the minimal GitHub source capability used by Run.
type sourceClient interface {
	sourceFileFetcher
	ResolveSelector(ctx context.Context, repo string, sel selector.Selector) (githubapi.ResolvedRef, error)
}
