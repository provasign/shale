package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/provasign/shale/internal/forge"
	"github.com/provasign/shale/internal/gitx"
	"github.com/provasign/shale/internal/prcard"
	"github.com/provasign/shale/internal/render"
	"github.com/provasign/shale/internal/store"
)

// cmdRender is the CI entry point (env contract per architecture §3.7) and
// the local preview.
func cmdRender(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("render", flag.ContinueOnError)
	fs.SetOutput(stderr)
	local := fs.Bool("local", false, "print the card to the terminal (no posting)")
	pr := fs.Int("pr", 0, "PR number to render and post to")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	if *local {
		return renderLocal(stdout, stderr)
	}

	prNum := *pr
	if prNum == 0 {
		prNum = detectPRNumber()
	}
	if prNum == 0 {
		fmt.Fprintln(stderr, "shale render: --pr <n> required (or run on a GitHub Actions pull_request event)")
		return 1
	}

	f, err := forge.New(forge.FromEnv())
	if err != nil {
		fmt.Fprintln(stderr, "shale render:", err)
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	res, err := prcard.Run(ctx, f, prNum)
	if err != nil {
		fmt.Fprintln(stderr, "shale render:", err)
		return 1
	}
	if res.Nudged {
		fmt.Fprintf(stdout, "shale: no evidence on PR #%d — nudge posted\n", prNum)
	} else {
		fmt.Fprintf(stdout, "shale: card posted to PR #%d (%d session(s))\n", prNum, res.Shales)
	}
	for _, w := range res.Tampered {
		fmt.Fprintln(stdout, "shale: ⚠️ ", w)
	}
	return 0
}

// renderLocal previews the card from the working tree: committed shale files
// plus the locally-changed file list.
func renderLocal(stdout, stderr io.Writer) int {
	root := repoRoot()
	entries, err := os.ReadDir(filepath.Join(root, ".shale"))
	if err != nil {
		fmt.Fprintln(stderr, "shale render: no .shale/ directory — run `shale init` first")
		return 1
	}
	var shales []*store.Shale
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") || e.Name() == "config.yaml" {
			continue
		}
		s, err := store.Load(filepath.Join(root, ".shale", e.Name()))
		if err != nil {
			fmt.Fprintf(stderr, "shale render: skipping %s: %v\n", e.Name(), err)
			continue
		}
		shales = append(shales, s)
	}

	// Local preview diff: files the sessions claim, plus anything currently
	// changed per git — close enough for a terminal preview.
	seen := map[string]bool{}
	var prFiles []render.ChangedFile
	for _, s := range shales {
		for _, f := range s.Files {
			if !seen[f.Path] {
				seen[f.Path] = true
				prFiles = append(prFiles, render.ChangedFile{Path: f.Path, Status: "modified"})
			}
		}
	}
	for _, p := range gitx.FilesChangedSince(root, time.Time{}) {
		if !seen[p] {
			seen[p] = true
			prFiles = append(prFiles, render.ChangedFile{Path: p, Status: "modified"})
		}
	}

	fmt.Fprintln(stdout, render.Card(render.Input{Shales: shales, PRFiles: prFiles}))
	return 0
}

// detectPRNumber reads the GitHub Actions event payload (plan D3).
func detectPRNumber() int {
	path := os.Getenv("GITHUB_EVENT_PATH")
	if path == "" {
		return 0
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	var event struct {
		PullRequest struct {
			Number int `json:"number"`
		} `json:"pull_request"`
		Number int `json:"number"`
	}
	if json.Unmarshal(raw, &event) != nil {
		return 0
	}
	if event.PullRequest.Number > 0 {
		return event.PullRequest.Number
	}
	return event.Number
}
