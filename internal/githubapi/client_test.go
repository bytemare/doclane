package githubapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bytemare/doclane/internal/selector"
)

func TestFetchFile(t *testing.T) {
	t.Parallel()

	var sawAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/acme/shared/contents/docs/guide.md" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("ref"); got != "abc123" {
			t.Fatalf("unexpected ref query: %q", got)
		}
		sawAuth = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"type":     "file",
			"encoding": "base64",
			"content":  base64.StdEncoding.EncodeToString([]byte("hello doclane")),
		})
	}))
	defer srv.Close()

	client := NewWithHTTPClient("token123", srv.Client(), srv.URL)
	got, err := client.FetchFile(context.Background(), "acme/shared", "docs/guide.md", "abc123")
	if err != nil {
		t.Fatalf("FetchFile returned error: %v", err)
	}
	if string(got) != "hello doclane" {
		t.Fatalf("unexpected file contents: %q", string(got))
	}
	if sawAuth != "Bearer token123" {
		t.Fatalf("expected Authorization header, got %q", sawAuth)
	}
}

func TestResolveSelectorLatest(t *testing.T) {
	t.Parallel()

	const commitSHA = "0123456789abcdef0123456789abcdef01234567"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/acme/shared":
			_ = json.NewEncoder(w).Encode(map[string]string{"default_branch": "main"})
		case "/repos/acme/shared/branches/main":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"commit": map[string]any{
					"sha": commitSHA,
					"commit": map[string]any{
						"committer": map[string]any{
							"date": "2026-02-24T18:00:00Z",
						},
					},
				},
			})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	client := NewWithHTTPClient("t", srv.Client(), srv.URL)
	sel, err := selector.Parse("latest")
	if err != nil {
		t.Fatalf("selector parse failed: %v", err)
	}

	resolved, err := client.ResolveSelector(context.Background(), "acme/shared", sel)
	if err != nil {
		t.Fatalf("ResolveSelector returned error: %v", err)
	}
	if resolved.CommitSHA != commitSHA {
		t.Fatalf("unexpected commit sha: %s", resolved.CommitSHA)
	}
	if resolved.ResolvedSelector != "branch:main" {
		t.Fatalf("unexpected resolved selector: %q", resolved.ResolvedSelector)
	}
}

func TestCreateOrGetPullRequestFallsBackToExisting(t *testing.T) {
	t.Parallel()

	var postSeen, getSeen, patchSeen bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/repos/acme/consumer/pulls":
			postSeen = true
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"message":"A pull request already exists for acme:branch."}`))
		case r.Method == http.MethodGet && r.URL.Path == "/repos/acme/consumer/pulls":
			getSeen = true
			if got := r.URL.Query().Get("head"); got != "acme:doclane/abc" {
				t.Fatalf("unexpected head query: %q", got)
			}
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"number": 42, "html_url": "https://github.com/acme/consumer/pull/42", "title": "existing"},
			})
		case r.Method == http.MethodPatch && r.URL.Path == "/repos/acme/consumer/pulls/42":
			patchSeen = true
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode patch body: %v", err)
			}
			if body["title"] != "test" || body["body"] != "body" {
				t.Fatalf("unexpected patch body: %#v", body)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"number":   42,
				"html_url": "https://github.com/acme/consumer/pull/42",
				"title":    "test",
			})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer srv.Close()

	client := NewWithHTTPClient("t", srv.Client(), srv.URL)
	pr, err := client.CreateOrGetPullRequest(context.Background(), "acme/consumer", PullRequestRequest{
		Title: "test",
		Head:  "doclane/abc",
		Base:  "main",
		Body:  "body",
	})
	if err != nil {
		t.Fatalf("CreateOrGetPullRequest returned error: %v", err)
	}
	if !postSeen || !getSeen || !patchSeen {
		t.Fatalf("expected POST, fallback GET, and PATCH to be called")
	}
	if pr.Number != 42 || !strings.Contains(pr.URL, "/pull/42") {
		t.Fatalf("unexpected PR returned: %+v", pr)
	}
	if pr.Title != "test" {
		t.Fatalf("expected updated PR title, got %q", pr.Title)
	}
}

func TestNewWithHTTPClientDefaults(t *testing.T) {
	t.Parallel()

	client := NewWithHTTPClient("  token  ", nil, "")
	if client.http == nil {
		t.Fatal("expected default HTTP client")
	}
	if client.baseURL != "https://api.github.com" {
		t.Fatalf("unexpected default baseURL: %q", client.baseURL)
	}
	if client.token != "token" {
		t.Fatalf("unexpected token trim behavior: %q", client.token)
	}

	client = New("abc")
	if client.token != "abc" {
		t.Fatalf("unexpected token in New: %q", client.token)
	}
}

func TestResolveSelectorTagAnnotated(t *testing.T) {
	t.Parallel()

	const (
		tagName   = "v1.2.3"
		tagObjSHA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		commitSHA = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/acme/shared/git/ref/tags/" + tagName:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"object": map[string]string{
					"type": "tag",
					"sha":  tagObjSHA,
				},
			})
		case "/repos/acme/shared/git/tags/" + tagObjSHA:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"object": map[string]string{
					"type": "commit",
					"sha":  commitSHA,
				},
				"verification": map[string]any{
					"verified": true,
					"reason":   "valid",
				},
			})
		case "/repos/acme/shared/commits/" + commitSHA:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"commit": map[string]any{
					"committer": map[string]any{
						"date": "2026-02-27T12:00:00Z",
					},
				},
			})
		default:
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	client := NewWithHTTPClient("t", srv.Client(), srv.URL)
	sel, err := selector.Parse("tag:" + tagName)
	if err != nil {
		t.Fatalf("selector parse failed: %v", err)
	}
	resolved, err := client.ResolveSelector(context.Background(), "acme/shared", sel)
	if err != nil {
		t.Fatalf("ResolveSelector returned error: %v", err)
	}
	if resolved.CommitSHA != commitSHA {
		t.Fatalf("unexpected commit SHA: %q", resolved.CommitSHA)
	}
	if resolved.TagVerification == nil || !resolved.TagVerification.Verified || resolved.TagVerification.Reason != "valid" {
		t.Fatalf("unexpected tag verification: %+v", resolved.TagVerification)
	}
}

func TestResolveSelectorTagLightweight(t *testing.T) {
	t.Parallel()

	const commitSHA = "cccccccccccccccccccccccccccccccccccccccc"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/acme/shared/git/ref/tags/v1":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"object": map[string]string{
					"type": "commit",
					"sha":  commitSHA,
				},
			})
		case "/repos/acme/shared/commits/" + commitSHA:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"commit": map[string]any{
					"committer": map[string]any{
						"date": "2026-02-27T12:00:00Z",
					},
				},
			})
		default:
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	client := NewWithHTTPClient("t", srv.Client(), srv.URL)
	sel, _ := selector.Parse("tag:v1")
	resolved, err := client.ResolveSelector(context.Background(), "acme/shared", sel)
	if err != nil {
		t.Fatalf("ResolveSelector returned error: %v", err)
	}
	if resolved.TagVerification == nil || resolved.TagVerification.Reason != "lightweight_tag" {
		t.Fatalf("expected lightweight tag verification metadata, got %+v", resolved.TagVerification)
	}
}

func TestResolveSelectorTagUnsupportedObject(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/acme/shared/git/ref/tags/v1" {
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"object": map[string]string{
				"type": "tree",
				"sha":  "deadbeef",
			},
		})
	}))
	defer srv.Close()

	client := NewWithHTTPClient("t", srv.Client(), srv.URL)
	sel, _ := selector.Parse("tag:v1")
	if _, err := client.ResolveSelector(context.Background(), "acme/shared", sel); err == nil {
		t.Fatal("expected unsupported tag object type error")
	}
}

func TestGetDefaultBranchNoDefault(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{})
	}))
	defer srv.Close()

	client := NewWithHTTPClient("t", srv.Client(), srv.URL)
	if _, err := client.GetDefaultBranch(context.Background(), "acme/shared"); err == nil {
		t.Fatal("expected missing default branch error")
	}
}

func TestFetchFileValidationErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		response map[string]string
	}{
		{
			name: "not_file",
			response: map[string]string{
				"type":     "dir",
				"encoding": "base64",
				"content":  "SGk=",
			},
		},
		{
			name: "bad_encoding",
			response: map[string]string{
				"type":     "file",
				"encoding": "utf-8",
				"content":  "hello",
			},
		},
		{
			name: "invalid_base64",
			response: map[string]string{
				"type":     "file",
				"encoding": "base64",
				"content":  "%%%invalid%%%",
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(tt.response)
			}))
			defer srv.Close()

			client := NewWithHTTPClient("t", srv.Client(), srv.URL)
			if _, err := client.FetchFile(context.Background(), "acme/shared", "README.md", "abc"); err == nil {
				t.Fatal("expected FetchFile validation error")
			}
		})
	}
}

func TestDoJSONAndHelpers(t *testing.T) {
	t.Parallel()

	client := NewWithHTTPClient("", &http.Client{}, "https://api.github.com")
	if err := client.getJSON(context.Background(), "/repos/acme/shared", &struct{}{}); err == nil {
		t.Fatal("expected token required error")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/not-found":
			http.NotFound(w, r)
		case "/http-error":
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"message":"bad request"}`)
		case "/no-output":
			_, _ = io.WriteString(w, "{}")
		case "/decode-error":
			_, _ = io.WriteString(w, "not json")
		default:
			_ = json.NewEncoder(w).Encode(map[string]string{"ok": "true"})
		}
	}))
	defer srv.Close()

	client = NewWithHTTPClient("t", srv.Client(), srv.URL)
	if err := client.getJSON(context.Background(), "/not-found", &struct{}{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	err := client.doJSON(context.Background(), http.MethodGet, "/http-error", nil, &struct{}{})
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("expected HTTPError, got %T", err)
	}
	if !strings.Contains(httpErr.Error(), "400") {
		t.Fatalf("unexpected HTTPError text: %v", httpErr)
	}
	if err := client.doJSON(context.Background(), http.MethodGet, "/no-output", nil, nil); err != nil {
		t.Fatalf("expected nil output path to pass, got %v", err)
	}
	if err := client.doJSON(context.Background(), http.MethodGet, "/decode-error", nil, &struct{}{}); err == nil {
		t.Fatal("expected decode error")
	}
}

func TestHelperFunctions(t *testing.T) {
	t.Parallel()

	if !isValidationConflict(&HTTPError{
		StatusCode: http.StatusUnprocessableEntity,
		Body:       "A pull request already exists",
	}) {
		t.Fatal("expected validation conflict detection")
	}
	if isValidationConflict(errors.New("x")) {
		t.Fatal("non-HTTP errors should not be validation conflicts")
	}

	if _, _, err := splitRepo("acme"); err == nil {
		t.Fatal("expected splitRepo error for invalid repo")
	}
	owner, name, err := splitRepo("acme/shared")
	if err != nil {
		t.Fatalf("splitRepo returned error: %v", err)
	}
	if owner != "acme" || name != "shared" {
		t.Fatalf("unexpected splitRepo values: %s / %s", owner, name)
	}

	if got := escapeRepoPath("/docs/My File.md"); got != "docs/My%20File.md" {
		t.Fatalf("unexpected escaped path: %q", got)
	}

	if got := (&HTTPError{StatusCode: 500}).Error(); got != "github API error: 500" {
		t.Fatalf("unexpected HTTPError string: %q", got)
	}
	if got := (&HTTPError{StatusCode: 500, Body: "x"}).Error(); got != "github API error: 500: x" {
		t.Fatalf("unexpected HTTPError string with body: %q", got)
	}
}

func TestResolveSelectorCommitErrorPath(t *testing.T) {
	t.Parallel()

	const commitSHA = "0123456789abcdef0123456789abcdef01234567"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != fmt.Sprintf("/repos/acme/shared/commits/%s", commitSHA) {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"commit":{"committer":{"date":"not-a-date"}}}`)
	}))
	defer srv.Close()

	client := NewWithHTTPClient("t", srv.Client(), srv.URL)
	sel, _ := selector.Parse("commit:" + commitSHA)
	if _, err := client.ResolveSelector(context.Background(), "acme/shared", sel); err == nil {
		t.Fatal("expected commit-date decode error")
	}
}
