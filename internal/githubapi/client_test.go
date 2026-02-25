package githubapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
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
