package syncer

import (
	"context"
	"testing"

	"github.com/bytemare/doclane/internal/config"
)

type fakeFileFetcher struct {
	files map[string][]byte
}

func (f fakeFileFetcher) FetchFile(_ context.Context, _ string, filePath, _ string) ([]byte, error) {
	if data, ok := f.files[filePath]; ok {
		cp := make([]byte, len(data))
		copy(cp, data)
		return cp, nil
	}
	return nil, nil
}

func TestStageFilesMissingTargetEmptyFileCountsAsChanged(t *testing.T) {
	t.Parallel()

	workdir := t.TempDir()
	cfg := &config.Config{
		Source: config.SourceConfig{Repo: "acme/shared"},
		Sync: []config.SyncEntry{
			{ID: "empty", Source: "docs/EMPTY.md", Target: "docs/EMPTY.md"},
		},
	}
	staged, results, err := stageFiles(context.Background(), fakeFileFetcher{
		files: map[string][]byte{"docs/EMPTY.md": {}},
	}, cfg, "deadbeef", workdir, true)
	if err != nil {
		t.Fatalf("stageFiles returned error: %v", err)
	}
	if len(staged) != 1 || len(results) != 1 {
		t.Fatalf("unexpected staged/results lengths: %d/%d", len(staged), len(results))
	}
	if !staged[0].changed || !results[0].Changed {
		t.Fatal("expected missing target with empty upstream file to be treated as changed")
	}
}
