// Package forge abstracts the git hosting platform (ADR D10): the renderer
// talks to this interface only, so the same binary works from GitHub
// Actions, Jenkins, or any CI. All evidence/diff acquisition is API-based —
// never a code checkout (architecture §3.7).
package forge

import (
	"context"
	"fmt"
	"os"

	"github.com/provasign/shale/internal/render"
)

// Forge is the driver interface (architecture §3.4). Drivers: github
// (MVP 1), gitlab (MVP 2), bitbucket (backlog).
type Forge interface {
	// PRHead returns the head SHA of the pull request.
	PRHead(ctx context.Context, pr int) (string, error)
	// PRFiles returns the changed files of the pull request (the diff, via API).
	PRFiles(ctx context.Context, pr int) ([]render.ChangedFile, error)
	// ListDir lists file paths under dir at ref (no checkout).
	ListDir(ctx context.Context, ref, dir string) ([]string, error)
	// FileContent fetches one file's bytes at ref.
	FileContent(ctx context.Context, ref, path string) ([]byte, error)
	// PRCommits returns the commit SHAs of the PR, oldest first.
	PRCommits(ctx context.Context, pr int) ([]string, error)
	// CommitsTouching returns the SHAs (within ref's history) that touched path.
	CommitsTouching(ctx context.Context, ref, path string) ([]string, error)
	// UpsertComment creates or edits-in-place the PR comment carrying marker.
	UpsertComment(ctx context.Context, pr int, marker, body string) error
	// SetStatus posts a non-blocking status/check for the head SHA.
	// conclusion is "neutral" or "success" — never "failure" (ADR D5).
	SetStatus(ctx context.Context, headSHA, conclusion, title, summary string) error
	// BlobBase returns the base URL for viewing blob files at head, e.g.
	// "https://github.com/owner/repo/blob/<sha>". Returns "" if unknown.
	BlobBase(head string) string
}

// Config carries the env contract (architecture §3.7).
type Config struct {
	Forge  string // SHALE_FORGE: github (default) | gitlab
	Token  string // SHALE_TOKEN, falls back to GITHUB_TOKEN
	Repo   string // SHALE_REPO: owner/repo
	APIURL string // override for tests / GHES
}

// FromEnv builds Config from the documented environment contract, with
// GitHub Actions auto-detection.
func FromEnv() Config {
	cfg := Config{
		Forge: getenv("SHALE_FORGE", "github"),
		Token: os.Getenv("SHALE_TOKEN"),
		Repo:  os.Getenv("SHALE_REPO"),
	}
	if cfg.Token == "" {
		cfg.Token = os.Getenv("GITHUB_TOKEN")
	}
	if cfg.Repo == "" {
		cfg.Repo = os.Getenv("GITHUB_REPOSITORY")
	}
	return cfg
}

// New returns the driver for cfg.
func New(cfg Config) (Forge, error) {
	switch cfg.Forge {
	case "github", "":
		return NewGitHub(cfg)
	default:
		return nil, fmt.Errorf("unsupported forge %q (github in MVP 1; gitlab lands in MVP 2)", cfg.Forge)
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
