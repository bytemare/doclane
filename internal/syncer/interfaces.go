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
