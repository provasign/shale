package forge

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestGitHub(t *testing.T, handler http.HandlerFunc) *GitHub {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	g, err := NewGitHub(Config{Repo: "acme/widgets", Token: "t0ken", APIURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func TestNewGitHubValidation(t *testing.T) {
	if _, err := NewGitHub(Config{Token: "t"}); err == nil {
		t.Fatal("missing repo must error")
	}
	if _, err := NewGitHub(Config{Repo: "a/b"}); err == nil {
		t.Fatal("missing token must error")
	}
	if _, err := New(Config{Forge: "gitlab", Repo: "a/b", Token: "t"}); err == nil {
		t.Fatal("gitlab driver is MVP 2 — must error clearly")
	}
}

func TestPRHeadAndAuth(t *testing.T) {
	g := newTestGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer t0ken" {
			t.Errorf("missing auth header")
		}
		if r.URL.Path != "/repos/acme/widgets/pulls/7" {
			t.Errorf("path = %s", r.URL.Path)
		}
		fmt.Fprint(w, `{"head":{"sha":"abc123"}}`)
	})
	sha, err := g.PRHead(context.Background(), 7)
	if err != nil || sha != "abc123" {
		t.Fatalf("sha=%q err=%v", sha, err)
	}
}

func TestPRFilesPagination(t *testing.T) {
	g := newTestGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		var files []map[string]string
		n := 100
		if page == "2" {
			n = 3
		}
		for i := 0; i < n; i++ {
			files = append(files, map[string]string{
				"filename": fmt.Sprintf("f%s_%d.go", page, i), "status": "modified",
			})
		}
		json.NewEncoder(w).Encode(files)
	})
	files, err := g.PRFiles(context.Background(), 7)
	if err != nil || len(files) != 103 {
		t.Fatalf("len=%d err=%v", len(files), err)
	}
}

func TestListDirMissingIsNotError(t *testing.T) {
	g := newTestGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
	})
	paths, err := g.ListDir(context.Background(), "head", ".shale")
	if err != nil || paths != nil {
		t.Fatalf("missing dir must be nil,nil — got %v, %v", paths, err)
	}
}

func TestFileContentBase64(t *testing.T) {
	g := newTestGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		content := base64.StdEncoding.EncodeToString([]byte("hello shale"))
		json.NewEncoder(w).Encode(map[string]string{"content": content, "encoding": "base64"})
	})
	b, err := g.FileContent(context.Background(), "head", ".shale/x.yaml")
	if err != nil || string(b) != "hello shale" {
		t.Fatalf("b=%q err=%v", b, err)
	}
}

func TestUpsertCommentEditsInPlace(t *testing.T) {
	var patched bool
	g := newTestGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET":
			json.NewEncoder(w).Encode([]map[string]any{
				{"id": 11, "body": "unrelated"},
				{"id": 22, "body": "<!-- shale-card -->\nold card"},
			})
		case r.Method == "PATCH" && strings.HasSuffix(r.URL.Path, "/comments/22"):
			patched = true
			fmt.Fprint(w, `{}`)
		case r.Method == "POST":
			t.Error("must PATCH the existing comment, not POST a new one")
		}
	})
	if err := g.UpsertComment(context.Background(), 7, "<!-- shale-card -->", "new card"); err != nil {
		t.Fatal(err)
	}
	if !patched {
		t.Fatal("existing comment not edited in place")
	}
}

func TestUpsertNudgePostedAtMostOnce(t *testing.T) {
	g := newTestGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			json.NewEncoder(w).Encode([]map[string]any{
				{"id": 5, "body": "<!-- shale-nudge -->\nexisting nudge"},
			})
			return
		}
		t.Errorf("nudge must never be re-posted or edited: %s %s", r.Method, r.URL.Path)
	})
	if err := g.UpsertComment(context.Background(), 7, "<!-- shale-nudge -->", "nudge"); err != nil {
		t.Fatal(err)
	}
}

func TestSetStatusNeverFailing(t *testing.T) {
	var got map[string]any
	g := newTestGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&got)
		fmt.Fprint(w, `{}`)
	})
	// Even if a caller passes "failure", the driver coerces to neutral (ADR D5).
	if err := g.SetStatus(context.Background(), "sha", "failure", "t", "s"); err != nil {
		t.Fatal(err)
	}
	if got["conclusion"] != "neutral" {
		t.Fatalf("conclusion = %v — the Check Run must never fail", got["conclusion"])
	}
}

func TestRateLimitRetry(t *testing.T) {
	attempts := 0
	g := newTestGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.Header().Set("Retry-After", "0")
			http.Error(w, `{"message":"rate limited"}`, http.StatusForbidden)
			return
		}
		fmt.Fprint(w, `{"head":{"sha":"abc"}}`)
	})
	sha, err := g.PRHead(context.Background(), 1)
	if err != nil || sha != "abc" || attempts != 2 {
		t.Fatalf("sha=%q attempts=%d err=%v", sha, attempts, err)
	}
}

func TestFromEnv(t *testing.T) {
	t.Setenv("SHALE_FORGE", "")
	t.Setenv("SHALE_TOKEN", "")
	t.Setenv("SHALE_REPO", "")
	t.Setenv("GITHUB_TOKEN", "gh-tok")
	t.Setenv("GITHUB_REPOSITORY", "acme/widgets")
	cfg := FromEnv()
	if cfg.Forge != "github" || cfg.Token != "gh-tok" || cfg.Repo != "acme/widgets" {
		t.Fatalf("cfg = %+v", cfg)
	}
	t.Setenv("SHALE_TOKEN", "explicit")
	if FromEnv().Token != "explicit" {
		t.Fatal("SHALE_TOKEN must win over GITHUB_TOKEN")
	}
}

func TestPRCommitsAndCommitsTouching(t *testing.T) {
	g := newTestGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/pulls/7/commits"):
			json.NewEncoder(w).Encode([]map[string]string{{"sha": "c1"}, {"sha": "c2"}})
		case strings.Contains(r.URL.Path, "/commits"):
			if r.URL.Query().Get("path") != ".shale/x.yaml" {
				t.Errorf("path param = %q", r.URL.Query().Get("path"))
			}
			json.NewEncoder(w).Encode([]map[string]string{{"sha": "c2"}})
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	})
	commits, err := g.PRCommits(context.Background(), 7)
	if err != nil || len(commits) != 2 || commits[0] != "c1" {
		t.Fatalf("commits=%v err=%v", commits, err)
	}
	touching, err := g.CommitsTouching(context.Background(), "head", ".shale/x.yaml")
	if err != nil || len(touching) != 1 || touching[0] != "c2" {
		t.Fatalf("touching=%v err=%v", touching, err)
	}
}

func TestSetStatusFallsBackToCommitStatus(t *testing.T) {
	var statusPosted bool
	g := newTestGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/check-runs") {
			// Two 403s: the first triggers the rate-limit retry, the second
			// proves a hard permission failure → commit-status fallback.
			http.Error(w, `{"message":"Resource not accessible"}`, http.StatusForbidden)
			return
		}
		if strings.Contains(r.URL.Path, "/statuses/") {
			statusPosted = true
			fmt.Fprint(w, `{}`)
			return
		}
		t.Errorf("unexpected path %s", r.URL.Path)
	})
	if err := g.SetStatus(context.Background(), "sha", "neutral", "title", "summary"); err != nil {
		t.Fatal(err)
	}
	if !statusPosted {
		t.Fatal("commit-status fallback not used on 403")
	}
}

func TestListDirFiltersDirectories(t *testing.T) {
	g := newTestGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]string{
			{"path": ".shale/a.yaml", "type": "file"},
			{"path": ".shale/transcripts", "type": "dir"},
			{"path": ".shale/b.yaml", "type": "file"},
		})
	})
	paths, err := g.ListDir(context.Background(), "head", ".shale")
	if err != nil || len(paths) != 2 {
		t.Fatalf("paths=%v err=%v", paths, err)
	}
}

func TestFileContentRejectsWeirdEncoding(t *testing.T) {
	g := newTestGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"content": "x", "encoding": "rot13"})
	})
	if _, err := g.FileContent(context.Background(), "head", "f"); err == nil {
		t.Fatal("unknown encoding must error")
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("hello", 10); got != "hello" {
		t.Fatalf("got %q", got)
	}
	if got := truncate("hello world", 5); got != "hello…" {
		t.Fatalf("got %q", got)
	}
}
