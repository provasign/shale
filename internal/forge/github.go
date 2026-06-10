package forge

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
	"strings"
	"time"

	"github.com/provasign/shale/internal/render"
)

// GitHub implements Forge over the REST v3 API.
type GitHub struct {
	apiURL string
	repo   string // owner/repo
	token  string
	client *http.Client
}

// NewGitHub validates config and returns the driver.
func NewGitHub(cfg Config) (*GitHub, error) {
	if cfg.Repo == "" || !strings.Contains(cfg.Repo, "/") {
		return nil, errors.New("SHALE_REPO (owner/repo) is required")
	}
	if cfg.Token == "" {
		return nil, errors.New("SHALE_TOKEN or GITHUB_TOKEN is required")
	}
	api := cfg.APIURL
	if api == "" {
		api = "https://api.github.com"
	}
	return &GitHub{
		apiURL: strings.TrimRight(api, "/"),
		repo:   cfg.Repo,
		token:  cfg.Token,
		client: &http.Client{Timeout: 30 * time.Second},
	}, nil
}

// do performs one API call with a single retry on 403/429 rate limiting.
func (g *GitHub) do(ctx context.Context, method, path string, body, out any) error {
	for attempt := 0; ; attempt++ {
		var rdr io.Reader
		if body != nil {
			raw, err := json.Marshal(body)
			if err != nil {
				return err
			}
			rdr = bytes.NewReader(raw)
		}
		req, err := http.NewRequestWithContext(ctx, method, g.apiURL+path, rdr)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+g.token)
		req.Header.Set("Accept", "application/vnd.github+json")
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := g.client.Do(req)
		if err != nil {
			return err
		}
		data, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
		resp.Body.Close()
		if err != nil {
			return err
		}
		if (resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests) && attempt == 0 {
			// One rate-limit-aware retry (plan D2); respect Retry-After if sane.
			delay := 2 * time.Second
			if ra := resp.Header.Get("Retry-After"); ra != "" {
				if d, perr := time.ParseDuration(ra + "s"); perr == nil && d < 30*time.Second {
					delay = d
				}
			}
			select {
			case <-time.After(delay):
				continue
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("github %s %s: %s: %s", method, path, resp.Status, truncate(string(data), 200))
		}
		if out != nil {
			return json.Unmarshal(data, out)
		}
		return nil
	}
}

func (g *GitHub) PRHead(ctx context.Context, pr int) (string, error) {
	var out struct {
		Head struct {
			SHA string `json:"sha"`
		} `json:"head"`
	}
	err := g.do(ctx, "GET", fmt.Sprintf("/repos/%s/pulls/%d", g.repo, pr), nil, &out)
	return out.Head.SHA, err
}

func (g *GitHub) PRFiles(ctx context.Context, pr int) ([]render.ChangedFile, error) {
	var files []render.ChangedFile
	for page := 1; ; page++ {
		var batch []struct {
			Filename string `json:"filename"`
			Status   string `json:"status"`
		}
		path := fmt.Sprintf("/repos/%s/pulls/%d/files?per_page=100&page=%d", g.repo, pr, page)
		if err := g.do(ctx, "GET", path, nil, &batch); err != nil {
			return nil, err
		}
		for _, f := range batch {
			files = append(files, render.ChangedFile{Path: f.Filename, Status: f.Status})
		}
		if len(batch) < 100 {
			return files, nil
		}
	}
}

func (g *GitHub) ListDir(ctx context.Context, ref, dir string) ([]string, error) {
	var batch []struct {
		Path string `json:"path"`
		Type string `json:"type"`
	}
	path := fmt.Sprintf("/repos/%s/contents/%s?ref=%s", g.repo, url.PathEscape(dir), url.QueryEscape(ref))
	if err := g.do(ctx, "GET", path, nil, &batch); err != nil {
		if strings.Contains(err.Error(), "404") {
			return nil, nil // no .shale/ directory — that's the nudge path, not an error
		}
		return nil, err
	}
	var paths []string
	for _, e := range batch {
		if e.Type == "file" {
			paths = append(paths, e.Path)
		}
	}
	return paths, nil
}

func (g *GitHub) FileContent(ctx context.Context, ref, path string) ([]byte, error) {
	var out struct {
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
	}
	p := fmt.Sprintf("/repos/%s/contents/%s?ref=%s", g.repo, url.PathEscape(path), url.QueryEscape(ref))
	if err := g.do(ctx, "GET", p, nil, &out); err != nil {
		return nil, err
	}
	if out.Encoding != "base64" {
		return nil, fmt.Errorf("unexpected content encoding %q for %s", out.Encoding, path)
	}
	return base64.StdEncoding.DecodeString(strings.ReplaceAll(out.Content, "\n", ""))
}

func (g *GitHub) PRCommits(ctx context.Context, pr int) ([]string, error) {
	var shas []string
	for page := 1; ; page++ {
		var batch []struct {
			SHA string `json:"sha"`
		}
		path := fmt.Sprintf("/repos/%s/pulls/%d/commits?per_page=100&page=%d", g.repo, pr, page)
		if err := g.do(ctx, "GET", path, nil, &batch); err != nil {
			return nil, err
		}
		for _, c := range batch {
			shas = append(shas, c.SHA)
		}
		if len(batch) < 100 {
			return shas, nil
		}
	}
}

func (g *GitHub) CommitsTouching(ctx context.Context, ref, path string) ([]string, error) {
	var batch []struct {
		SHA string `json:"sha"`
	}
	p := fmt.Sprintf("/repos/%s/commits?sha=%s&path=%s&per_page=100", g.repo, url.QueryEscape(ref), url.QueryEscape(path))
	if err := g.do(ctx, "GET", p, nil, &batch); err != nil {
		return nil, err
	}
	shas := make([]string, 0, len(batch))
	for _, c := range batch {
		shas = append(shas, c.SHA)
	}
	return shas, nil
}

func (g *GitHub) UpsertComment(ctx context.Context, pr int, marker, body string) error {
	// Find a prior comment by hidden marker (architecture §3.4: edit in
	// place, never spam).
	for page := 1; ; page++ {
		var batch []struct {
			ID   int64  `json:"id"`
			Body string `json:"body"`
		}
		path := fmt.Sprintf("/repos/%s/issues/%d/comments?per_page=100&page=%d", g.repo, pr, page)
		if err := g.do(ctx, "GET", path, nil, &batch); err != nil {
			return err
		}
		for _, c := range batch {
			if strings.Contains(c.Body, marker) {
				if marker == render.NudgeMarker {
					return nil // the nudge is posted at most once, never updated
				}
				return g.do(ctx, "PATCH", fmt.Sprintf("/repos/%s/issues/comments/%d", g.repo, c.ID),
					map[string]string{"body": body}, nil)
			}
		}
		if len(batch) < 100 {
			break
		}
	}
	return g.do(ctx, "POST", fmt.Sprintf("/repos/%s/issues/%d/comments", g.repo, pr),
		map[string]string{"body": body}, nil)
}

func (g *GitHub) SetStatus(ctx context.Context, headSHA, conclusion, title, summary string) error {
	if conclusion != "neutral" && conclusion != "success" {
		// ADR D5: the Check Run is never failing. Refuse at the driver level.
		conclusion = "neutral"
	}
	body := map[string]any{
		"name":       "shale",
		"head_sha":   headSHA,
		"status":     "completed",
		"conclusion": conclusion,
		"output": map[string]string{
			"title":   title,
			"summary": truncate(summary, 60000),
		},
	}
	err := g.do(ctx, "POST", fmt.Sprintf("/repos/%s/check-runs", g.repo), body, nil)
	if err != nil && strings.Contains(err.Error(), "403") {
		// Some tokens can't create check runs; fall back to a commit status
		// rather than failing the render (fail-open).
		return g.do(ctx, "POST", fmt.Sprintf("/repos/%s/statuses/%s", g.repo, headSHA),
			map[string]string{"state": "success", "context": "shale", "description": truncate(title, 130)}, nil)
	}
	return err
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
