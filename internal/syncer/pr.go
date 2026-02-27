package syncer

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/bytemare/doclane/internal/config"
	"github.com/bytemare/doclane/internal/githubapi"
	"github.com/bytemare/doclane/internal/gitops"
)

type gitClient interface {
	EnsureRepo() error
	SetUser(name, email string) error
	CurrentBranch() (string, error)
	CheckoutBranch(branch string) error
	Add(paths ...string) error
	Commit(message string, signoff, sign bool) error
	PushSetUpstream(branch string) error
	OriginRepo() (string, error)
}

type pullRequestClient interface {
	CreateOrGetPullRequest(ctx context.Context, repo string, req githubapi.PullRequestRequest) (githubapi.PullRequest, error)
}

var (
	newGitRunner = func(dir string) gitClient {
		return gitops.Runner{Dir: dir}
	}
	newPRClient = func(token string) pullRequestClient {
		return githubapi.New(token)
	}
)

// createPullRequestFlow commits and pushes changes, then creates or reuses a PR.
func createPullRequestFlow(ctx context.Context, opts Options, cfg *config.Config, resolved githubapi.ResolvedRef, files []FileResult, workdir string) (string, error) {
	git := newGitRunner(workdir)
	if err := git.EnsureRepo(); err != nil {
		return "", fmt.Errorf("create-pr requires a git repository checkout: %w", err)
	}
	if err := git.SetUser(opts.GitUserName, opts.GitUserEmail); err != nil {
		return "", err
	}

	base := opts.PRBase
	if base == "" {
		base = strings.TrimSpace(os.Getenv("GITHUB_REF_NAME"))
	}
	if base == "" {
		current, err := git.CurrentBranch()
		if err != nil {
			return "", err
		}
		if current != "" && current != "HEAD" {
			base = current
		}
	}
	if base == "" || base == "HEAD" {
		base = "main"
	}

	shortSHA := shortHash(resolved.CommitSHA)
	branchPrefix := strings.TrimSuffix(strings.TrimSpace(opts.PRBranchPrefix), "/")
	if branchPrefix == "" {
		branchPrefix = "chore/doclane-sync"
	}
	branch := fmt.Sprintf("%s/%s", branchPrefix, shortSHA)

	if err := git.CheckoutBranch(branch); err != nil {
		return "", err
	}

	addPaths := []string{opts.LockfilePath}
	for _, f := range files {
		if f.Changed {
			addPaths = append(addPaths, f.Target)
		}
	}
	if err := git.Add(addPaths...); err != nil {
		return "", err
	}

	commitMsg := fmt.Sprintf("chore(doclane): sync shared docs from %s@%s", cfg.Source.Repo, shortSHA)
	if err := git.Commit(commitMsg, opts.CommitSignoff, opts.CommitSign); err != nil {
		return "", err
	}
	if err := git.PushSetUpstream(branch); err != nil {
		return "", err
	}

	consumerRepo := strings.TrimSpace(os.Getenv("GITHUB_REPOSITORY"))
	if consumerRepo == "" {
		var err error
		consumerRepo, err = git.OriginRepo()
		if err != nil {
			return "", fmt.Errorf("determine consumer repo slug: %w", err)
		}
	}

	client := newPRClient(opts.GitHubToken)
	prTitle := fmt.Sprintf("%s from %s@%s", strings.TrimSpace(opts.PRTitlePrefix), cfg.Source.Repo, shortSHA)
	prBody := buildPRBody(cfg.Source.Repo, resolved, files)
	pr, err := client.CreateOrGetPullRequest(ctx, consumerRepo, githubapi.PullRequestRequest{
		Title: prTitle,
		Head:  branch,
		Base:  base,
		Body:  prBody,
	})
	if err != nil {
		return "", err
	}
	return pr.URL, nil
}

// buildPRBody renders a deterministic provenance table for review and audit logs.
func buildPRBody(sourceRepo string, resolved githubapi.ResolvedRef, files []FileResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Doclane Sync\n\n")
	fmt.Fprintf(&b, "- Source repo: `%s`\n", sourceRepo)
	fmt.Fprintf(&b, "- Selector: `%s`\n", resolved.Selector)
	fmt.Fprintf(&b, "- Resolved selector: `%s`\n", resolved.ResolvedSelector)
	fmt.Fprintf(&b, "- Commit SHA: `%s`\n", resolved.CommitSHA)
	if !resolved.CommitDate.IsZero() {
		fmt.Fprintf(&b, "- Commit date: `%s`\n", resolved.CommitDate.UTC().Format(time.RFC3339))
	}
	if resolved.TagVerification != nil {
		fmt.Fprintf(&b, "- Tag verified: `%t`", resolved.TagVerification.Verified)
		if resolved.TagVerification.Reason != "" {
			fmt.Fprintf(&b, " (%s)", resolved.TagVerification.Reason)
		}
		fmt.Fprintln(&b)
	}
	fmt.Fprintf(&b, "\n### Files\n\n")
	fmt.Fprintf(&b, "| ID | Source | Target | Templated | Upstream SHA256 | Rendered SHA256 |\n")
	fmt.Fprintf(&b, "|---|---|---|---:|---|---|\n")
	for _, f := range files {
		fmt.Fprintf(&b, "| %s | `%s` | `%s` | %t | `%s` | `%s` |\n", f.ID, f.Source, f.Target, f.Templated, shortHash(f.UpstreamSHA), shortHash(f.RenderedSHA))
	}
	return b.String()
}

func shortHash(s string) string {
	if len(s) <= 12 {
		return s
	}
	return s[:12]
}
