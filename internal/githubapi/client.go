package githubapi

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/bytemare/doclane/internal/selector"
)

var ErrNotFound = errors.New("not found")

// Client is a minimal GitHub API client used by Doclane sync operations.
type Client struct {
	http    *http.Client
	token   string
	baseURL string
}

// ResolvedRef captures an immutable commit resolution for a selector.
type ResolvedRef struct {
	Selector         string
	ResolvedSelector string
	CommitSHA        string
	CommitDate       time.Time
	TagVerification  *TagVerification
}

// TagVerification carries GitHub's tag verification metadata for annotated tags.
type TagVerification struct {
	Verified bool
	Reason   string
	Source   string
}

// PullRequest is the subset of PR fields Doclane needs from the GitHub API.
type PullRequest struct {
	Number int    `json:"number"`
	URL    string `json:"html_url"`
	Title  string `json:"title"`
}

// PullRequestRequest is the payload used to create a pull request.
type PullRequestRequest struct {
	Title string `json:"title"`
	Head  string `json:"head"`
	Base  string `json:"base"`
	Body  string `json:"body"`
}

// New constructs a GitHub API client for the provided token.
func New(token string) *Client {
	return NewWithHTTPClient(token, nil, "")
}

// NewWithHTTPClient constructs a GitHub API client with an injected HTTP client and base URL.
// This is primarily used for tests and GitHub Enterprise Server support.
func NewWithHTTPClient(token string, httpClient *http.Client, baseURL string) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	if strings.TrimSpace(baseURL) == "" {
		baseURL = "https://api.github.com"
	}
	return &Client{
		http:    httpClient,
		token:   strings.TrimSpace(token),
		baseURL: strings.TrimRight(baseURL, "/"),
	}
}

// ResolveSelector resolves a selector to an immutable commit and metadata.
func (c *Client) ResolveSelector(ctx context.Context, repo string, sel selector.Selector) (ResolvedRef, error) {
	switch sel.Kind {
	case selector.KindLatest:
		defaultBranch, err := c.GetDefaultBranch(ctx, repo)
		if err != nil {
			return ResolvedRef{}, err
		}
		resolved, err := c.resolveBranch(ctx, repo, defaultBranch)
		if err != nil {
			return ResolvedRef{}, err
		}
		resolved.Selector = sel.String()
		resolved.ResolvedSelector = "branch:" + defaultBranch
		return resolved, nil
	case selector.KindBranch:
		resolved, err := c.resolveBranch(ctx, repo, sel.Value)
		if err != nil {
			return ResolvedRef{}, err
		}
		resolved.Selector = sel.String()
		resolved.ResolvedSelector = sel.String()
		return resolved, nil
	case selector.KindTag:
		resolved, err := c.resolveTag(ctx, repo, sel.Value)
		if err != nil {
			return ResolvedRef{}, err
		}
		resolved.Selector = sel.String()
		resolved.ResolvedSelector = sel.String()
		return resolved, nil
	case selector.KindCommit:
		date, err := c.getCommitDate(ctx, repo, sel.Value)
		if err != nil {
			return ResolvedRef{}, err
		}
		return ResolvedRef{
			Selector:         sel.String(),
			ResolvedSelector: sel.String(),
			CommitSHA:        sel.Value,
			CommitDate:       date,
		}, nil
	default:
		return ResolvedRef{}, fmt.Errorf("unsupported selector kind %q", sel.Kind)
	}
}

// GetDefaultBranch returns the default branch name for a repository.
func (c *Client) GetDefaultBranch(ctx context.Context, repo string) (string, error) {
	var out struct {
		DefaultBranch string `json:"default_branch"`
	}
	if err := c.getJSON(ctx, fmt.Sprintf("/repos/%s", repo), &out); err != nil {
		return "", err
	}
	if out.DefaultBranch == "" {
		return "", fmt.Errorf("repo %s has no default branch", repo)
	}
	return out.DefaultBranch, nil
}

// FetchFile fetches a repository file as raw bytes at a specific ref or commit.
func (c *Client) FetchFile(ctx context.Context, repo, filePath, ref string) ([]byte, error) {
	escapedPath := escapeRepoPath(filePath)
	ep := fmt.Sprintf("/repos/%s/contents/%s?ref=%s", repo, escapedPath, url.QueryEscape(ref))
	var out struct {
		Type     string `json:"type"`
		Encoding string `json:"encoding"`
		Content  string `json:"content"`
	}
	if err := c.getJSON(ctx, ep, &out); err != nil {
		return nil, err
	}
	if out.Type != "file" {
		return nil, fmt.Errorf("path %q is not a file", filePath)
	}
	if out.Encoding != "base64" {
		return nil, fmt.Errorf("unsupported content encoding %q for %s", out.Encoding, filePath)
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(out.Content, "\n", ""))
	if err != nil {
		return nil, fmt.Errorf("decode base64 for %s: %w", filePath, err)
	}
	return decoded, nil
}

// CreateOrGetPullRequest creates a PR or returns an existing matching open PR.
func (c *Client) CreateOrGetPullRequest(ctx context.Context, repo string, req PullRequestRequest) (PullRequest, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return PullRequest{}, err
	}
	var pr PullRequest
	err = c.doJSON(ctx, http.MethodPost, fmt.Sprintf("/repos/%s/pulls", repo), bytes.NewReader(body), &pr)
	if err == nil {
		return pr, nil
	}
	if !isValidationConflict(err) {
		return PullRequest{}, err
	}
	found, ferr := c.findOpenPullRequestByHead(ctx, repo, req.Head, req.Base)
	if ferr != nil {
		return PullRequest{}, fmt.Errorf("create PR conflict and fallback lookup failed: %w", errors.Join(err, ferr))
	}
	updated, uerr := c.UpdatePullRequest(ctx, repo, found.Number, req.Title, req.Body)
	if uerr != nil {
		return PullRequest{}, fmt.Errorf("reused PR %d but failed to refresh title/body: %w", found.Number, uerr)
	}
	if updated.URL == "" {
		updated.URL = found.URL
	}
	if updated.Number == 0 {
		updated.Number = found.Number
	}
	if updated.Title == "" {
		updated.Title = found.Title
	}
	return updated, nil
}

// UpdatePullRequest updates a pull request title/body.
func (c *Client) UpdatePullRequest(ctx context.Context, repo string, number int, title, body string) (PullRequest, error) {
	payload := struct {
		Title string `json:"title"`
		Body  string `json:"body"`
	}{
		Title: title,
		Body:  body,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return PullRequest{}, err
	}
	var pr PullRequest
	ep := fmt.Sprintf("/repos/%s/pulls/%d", repo, number)
	if err := c.doJSON(ctx, http.MethodPatch, ep, bytes.NewReader(data), &pr); err != nil {
		return PullRequest{}, err
	}
	return pr, nil
}

func (c *Client) resolveBranch(ctx context.Context, repo, branch string) (ResolvedRef, error) {
	var out struct {
		Commit struct {
			SHA    string `json:"sha"`
			Commit struct {
				Committer struct {
					Date time.Time `json:"date"`
				} `json:"committer"`
			} `json:"commit"`
		} `json:"commit"`
	}
	ep := fmt.Sprintf("/repos/%s/branches/%s", repo, url.PathEscape(branch))
	if err := c.getJSON(ctx, ep, &out); err != nil {
		return ResolvedRef{}, err
	}
	return ResolvedRef{
		CommitSHA:  out.Commit.SHA,
		CommitDate: out.Commit.Commit.Committer.Date,
	}, nil
}

func (c *Client) resolveTag(ctx context.Context, repo, tag string) (ResolvedRef, error) {
	var refOut struct {
		Object struct {
			Type string `json:"type"`
			SHA  string `json:"sha"`
		} `json:"object"`
	}
	ep := fmt.Sprintf("/repos/%s/git/ref/tags/%s", repo, url.PathEscape(tag))
	if err := c.getJSON(ctx, ep, &refOut); err != nil {
		return ResolvedRef{}, err
	}

	var commitSHA string
	var tv *TagVerification
	switch refOut.Object.Type {
	case "commit":
		commitSHA = refOut.Object.SHA
		tv = &TagVerification{Verified: false, Reason: "lightweight_tag", Source: "github-api"}
	case "tag":
		var tagOut struct {
			Object struct {
				Type string `json:"type"`
				SHA  string `json:"sha"`
			} `json:"object"`
			Verification struct {
				Verified bool   `json:"verified"`
				Reason   string `json:"reason"`
			} `json:"verification"`
		}
		ep := fmt.Sprintf("/repos/%s/git/tags/%s", repo, refOut.Object.SHA)
		if err := c.getJSON(ctx, ep, &tagOut); err != nil {
			return ResolvedRef{}, err
		}
		if tagOut.Object.Type != "commit" {
			return ResolvedRef{}, fmt.Errorf("tag %q does not point to a commit (got %s)", tag, tagOut.Object.Type)
		}
		commitSHA = tagOut.Object.SHA
		tv = &TagVerification{
			Verified: tagOut.Verification.Verified,
			Reason:   tagOut.Verification.Reason,
			Source:   "github-api",
		}
	default:
		return ResolvedRef{}, fmt.Errorf("unsupported tag ref object type %q", refOut.Object.Type)
	}

	date, err := c.getCommitDate(ctx, repo, commitSHA)
	if err != nil {
		return ResolvedRef{}, err
	}
	return ResolvedRef{
		CommitSHA:       commitSHA,
		CommitDate:      date,
		TagVerification: tv,
	}, nil
}

func (c *Client) getCommitDate(ctx context.Context, repo, sha string) (time.Time, error) {
	var out struct {
		Commit struct {
			Committer struct {
				Date time.Time `json:"date"`
			} `json:"committer"`
		} `json:"commit"`
	}
	if err := c.getJSON(ctx, fmt.Sprintf("/repos/%s/commits/%s", repo, sha), &out); err != nil {
		return time.Time{}, err
	}
	return out.Commit.Committer.Date, nil
}

func (c *Client) findOpenPullRequestByHead(ctx context.Context, repo, head, base string) (PullRequest, error) {
	owner, _, err := splitRepo(repo)
	if err != nil {
		return PullRequest{}, err
	}
	headParam := owner + ":" + head
	ep := fmt.Sprintf("/repos/%s/pulls?state=open&head=%s&base=%s", repo, url.QueryEscape(headParam), url.QueryEscape(base))
	var prs []PullRequest
	if err := c.getJSON(ctx, ep, &prs); err != nil {
		return PullRequest{}, err
	}
	if len(prs) == 0 {
		return PullRequest{}, fmt.Errorf("no open pull request found for %s -> %s", head, base)
	}
	return prs[0], nil
}

func (c *Client) getJSON(ctx context.Context, endpoint string, out any) error {
	return c.doJSON(ctx, http.MethodGet, endpoint, nil, out)
}

func (c *Client) doJSON(ctx context.Context, method, endpoint string, body io.Reader, out any) error {
	if c.token == "" {
		return errors.New("github token is required")
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+endpoint, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		payload, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
		return &HTTPError{
			StatusCode: resp.StatusCode,
			Body:       strings.TrimSpace(string(payload)),
		}
	}
	if out == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

type HTTPError struct {
	StatusCode int
	Body       string
}

// Error implements the error interface.
func (e *HTTPError) Error() string {
	if e.Body == "" {
		return fmt.Sprintf("github API error: %d", e.StatusCode)
	}
	return fmt.Sprintf("github API error: %d: %s", e.StatusCode, e.Body)
}

func isValidationConflict(err error) bool {
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) {
		return false
	}
	return httpErr.StatusCode == http.StatusUnprocessableEntity && strings.Contains(strings.ToLower(httpErr.Body), "pull request")
}

func splitRepo(repo string) (string, string, error) {
	owner, name, ok := strings.Cut(repo, "/")
	if !ok || owner == "" || name == "" {
		return "", "", fmt.Errorf("invalid repo %q", repo)
	}
	return owner, name, nil
}

func escapeRepoPath(p string) string {
	clean := path.Clean(strings.TrimPrefix(p, "/"))
	if clean == "." {
		return ""
	}
	parts := strings.Split(clean, "/")
	for i := range parts {
		parts[i] = url.PathEscape(parts[i])
	}
	return strings.Join(parts, "/")
}
