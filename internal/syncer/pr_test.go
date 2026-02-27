package syncer

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/bytemare/doclane/internal/config"
	"github.com/bytemare/doclane/internal/githubapi"
)

type fakeGitClient struct {
	currentBranch string
	originRepo    string

	errEnsure   error
	errSetUser  error
	errCurrent  error
	errCheckout error
	errAdd      error
	errCommit   error
	errPush     error
	errOrigin   error

	gotCheckout string
	gotAdd      []string
	gotCommit   string
}

func (f *fakeGitClient) EnsureRepo() error {
	return f.errEnsure
}

func (f *fakeGitClient) SetUser(_, _ string) error {
	return f.errSetUser
}

func (f *fakeGitClient) CurrentBranch() (string, error) {
	if f.errCurrent != nil {
		return "", f.errCurrent
	}
	return f.currentBranch, nil
}

func (f *fakeGitClient) CheckoutBranch(branch string) error {
	f.gotCheckout = branch
	return f.errCheckout
}

func (f *fakeGitClient) Add(paths ...string) error {
	f.gotAdd = append([]string(nil), paths...)
	return f.errAdd
}

func (f *fakeGitClient) Commit(message string, _, _ bool) error {
	f.gotCommit = message
	return f.errCommit
}

func (f *fakeGitClient) PushSetUpstream(_ string) error {
	return f.errPush
}

func (f *fakeGitClient) OriginRepo() (string, error) {
	if f.errOrigin != nil {
		return "", f.errOrigin
	}
	return f.originRepo, nil
}

type fakePullRequestClient struct {
	gotRepo string
	gotReq  githubapi.PullRequestRequest
	pr      githubapi.PullRequest
	err     error
}

func (f *fakePullRequestClient) CreateOrGetPullRequest(_ context.Context, repo string, req githubapi.PullRequestRequest) (githubapi.PullRequest, error) {
	f.gotRepo = repo
	f.gotReq = req
	return f.pr, f.err
}

func TestCreatePullRequestFlow(t *testing.T) {
	oldGitFactory := newGitRunner
	oldPRFactory := newPRClient
	oldRepoEnv := os.Getenv("GITHUB_REPOSITORY")
	oldRefName := os.Getenv("GITHUB_REF_NAME")
	t.Cleanup(func() {
		newGitRunner = oldGitFactory
		newPRClient = oldPRFactory
		_ = os.Setenv("GITHUB_REPOSITORY", oldRepoEnv)
		_ = os.Setenv("GITHUB_REF_NAME", oldRefName)
	})

	fakeGit := &fakeGitClient{
		currentBranch: "feature/current",
		originRepo:    "acme/consumer",
	}
	fakePR := &fakePullRequestClient{
		pr: githubapi.PullRequest{URL: "https://github.com/acme/consumer/pull/123"},
	}
	newGitRunner = func(string) gitClient { return fakeGit }
	newPRClient = func(string) pullRequestClient { return fakePR }

	if err := os.Setenv("GITHUB_REPOSITORY", "acme/consumer"); err != nil {
		t.Fatalf("set env: %v", err)
	}
	if err := os.Setenv("GITHUB_REF_NAME", "release"); err != nil {
		t.Fatalf("set env: %v", err)
	}

	cfg := &config.Config{
		Source: config.SourceConfig{Repo: "acme/shared"},
	}
	resolved := githubapi.ResolvedRef{
		Selector:         "tag:v1.2.3",
		ResolvedSelector: "tag:v1.2.3",
		CommitSHA:        "0123456789abcdef0123456789abcdef01234567",
		CommitDate:       time.Date(2026, 2, 27, 1, 2, 3, 0, time.UTC),
	}
	files := []FileResult{
		{ID: "a", Source: "A.md", Target: "docs/A.md", Changed: true, UpstreamSHA: "u1", RenderedSHA: "r1"},
		{ID: "b", Source: "B.md", Target: "docs/B.md", Changed: false, UpstreamSHA: "u2", RenderedSHA: "r2"},
	}
	opts := Options{
		LockfilePath:   ".github/.doclane.lock.yml",
		PRBranchPrefix: "chore/doclane-sync",
		PRTitlePrefix:  "chore(doclane): sync shared docs",
		GitHubToken:    "token",
	}

	prURL, err := createPullRequestFlow(context.Background(), opts, cfg, resolved, files, t.TempDir())
	if err != nil {
		t.Fatalf("createPullRequestFlow returned error: %v", err)
	}
	if prURL != "https://github.com/acme/consumer/pull/123" {
		t.Fatalf("unexpected PR URL: %q", prURL)
	}
	if fakeGit.gotCheckout != "chore/doclane-sync/0123456789ab" {
		t.Fatalf("unexpected checkout branch: %q", fakeGit.gotCheckout)
	}
	if len(fakeGit.gotAdd) != 2 || fakeGit.gotAdd[0] != ".github/.doclane.lock.yml" || fakeGit.gotAdd[1] != "docs/A.md" {
		t.Fatalf("unexpected add paths: %v", fakeGit.gotAdd)
	}
	if !strings.Contains(fakeGit.gotCommit, "acme/shared@0123456789ab") {
		t.Fatalf("unexpected commit message: %q", fakeGit.gotCommit)
	}
	if fakePR.gotRepo != "acme/consumer" {
		t.Fatalf("unexpected consumer repo: %q", fakePR.gotRepo)
	}
	if fakePR.gotReq.Base != "release" {
		t.Fatalf("unexpected PR base: %q", fakePR.gotReq.Base)
	}
	if !strings.Contains(fakePR.gotReq.Body, "Doclane Sync") {
		t.Fatalf("unexpected PR body: %q", fakePR.gotReq.Body)
	}
}

func TestCreatePullRequestFlowFallbacks(t *testing.T) {
	oldGitFactory := newGitRunner
	oldPRFactory := newPRClient
	oldRepoEnv := os.Getenv("GITHUB_REPOSITORY")
	oldRefName := os.Getenv("GITHUB_REF_NAME")
	t.Cleanup(func() {
		newGitRunner = oldGitFactory
		newPRClient = oldPRFactory
		_ = os.Setenv("GITHUB_REPOSITORY", oldRepoEnv)
		_ = os.Setenv("GITHUB_REF_NAME", oldRefName)
	})

	fakeGit := &fakeGitClient{
		currentBranch: "HEAD",
		originRepo:    "acme/from-origin",
	}
	fakePR := &fakePullRequestClient{
		pr: githubapi.PullRequest{URL: "https://github.com/acme/from-origin/pull/1"},
	}
	newGitRunner = func(string) gitClient { return fakeGit }
	newPRClient = func(string) pullRequestClient { return fakePR }
	_ = os.Unsetenv("GITHUB_REPOSITORY")
	_ = os.Unsetenv("GITHUB_REF_NAME")

	cfg := &config.Config{
		Source: config.SourceConfig{Repo: "acme/shared"},
	}
	resolved := githubapi.ResolvedRef{CommitSHA: "0123456789abcdef0123456789abcdef01234567"}
	opts := Options{
		LockfilePath:  ".github/.doclane.lock.yml",
		PRTitlePrefix: "prefix",
		GitHubToken:   "token",
	}
	_, err := createPullRequestFlow(context.Background(), opts, cfg, resolved, nil, t.TempDir())
	if err != nil {
		t.Fatalf("createPullRequestFlow returned error: %v", err)
	}
	if fakeGit.gotCheckout != "chore/doclane-sync/0123456789ab" {
		t.Fatalf("unexpected default branch prefix: %q", fakeGit.gotCheckout)
	}
	if fakePR.gotRepo != "acme/from-origin" {
		t.Fatalf("expected fallback to origin repo, got %q", fakePR.gotRepo)
	}
	if fakePR.gotReq.Base != "main" {
		t.Fatalf("expected main fallback base, got %q", fakePR.gotReq.Base)
	}
}

func TestCreatePullRequestFlowErrors(t *testing.T) {
	oldGitFactory := newGitRunner
	oldPRFactory := newPRClient
	t.Cleanup(func() {
		newGitRunner = oldGitFactory
		newPRClient = oldPRFactory
	})

	newGitRunner = func(string) gitClient {
		return &fakeGitClient{errEnsure: errors.New("not a repo")}
	}
	newPRClient = func(string) pullRequestClient {
		return &fakePullRequestClient{}
	}
	_, err := createPullRequestFlow(context.Background(), Options{}, &config.Config{}, githubapi.ResolvedRef{}, nil, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "create-pr requires a git repository checkout") {
		t.Fatalf("expected ensure repo failure, got %v", err)
	}
}

func TestBuildPRBodyAndShortHash(t *testing.T) {
	t.Parallel()

	resolved := githubapi.ResolvedRef{
		Selector:         "tag:v1.0.0",
		ResolvedSelector: "tag:v1.0.0",
		CommitSHA:        "0123456789abcdef0123456789abcdef01234567",
		CommitDate:       time.Date(2026, 2, 27, 9, 0, 0, 0, time.UTC),
		TagVerification: &githubapi.TagVerification{
			Verified: true,
			Reason:   "valid",
		},
	}
	body := buildPRBody("acme/shared", resolved, []FileResult{
		{
			ID:          "readme",
			Source:      "README.md",
			Target:      "docs/README.md",
			Templated:   true,
			UpstreamSHA: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			RenderedSHA: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		},
	})
	if !strings.Contains(body, "## Doclane Sync") {
		t.Fatalf("missing heading in PR body:\n%s", body)
	}
	if !strings.Contains(body, "Tag verified: `true` (valid)") {
		t.Fatalf("missing tag verification details:\n%s", body)
	}
	if !strings.Contains(body, "`aaaaaaaaaaaa`") || !strings.Contains(body, "`bbbbbbbbbbbb`") {
		t.Fatalf("expected short hashes in PR body:\n%s", body)
	}
	if got := shortHash("0123456789ab"); got != "0123456789ab" {
		t.Fatalf("shortHash should keep <= 12 chars, got %q", got)
	}
}
