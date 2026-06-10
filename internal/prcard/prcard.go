// Package prcard orchestrates the CI half: fetch evidence and diff via the
// forge API (no checkout), verify tamper-evidence, render, and post. It is
// driver-agnostic and tested against a fake Forge.
package prcard

import (
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/provasign/shale/internal/forge"
	"github.com/provasign/shale/internal/render"
	"github.com/provasign/shale/internal/store"
)

// Result reports what Run did, for logs and tests.
type Result struct {
	Card     string
	Shales   int
	Nudged   bool
	HeadSHA  string
	Tampered []string
}

// Run builds and posts the card for one PR.
func Run(ctx context.Context, f forge.Forge, pr int) (Result, error) {
	var res Result

	head, err := f.PRHead(ctx, pr)
	if err != nil {
		return res, fmt.Errorf("resolve PR head: %w", err)
	}
	res.HeadSHA = head

	prFiles, err := f.PRFiles(ctx, pr)
	if err != nil {
		return res, fmt.Errorf("list PR files: %w", err)
	}

	shales, tamper := loadShales(ctx, f, head, pr)
	res.Shales = len(shales)
	res.Tampered = tamper

	in := render.Input{Shales: shales, PRFiles: prFiles, TamperFlags: tamper}
	card := render.Card(in)
	res.Card = card

	if len(shales) == 0 {
		res.Nudged = true
		if err := f.UpsertComment(ctx, pr, render.NudgeMarker, card); err != nil {
			return res, fmt.Errorf("post nudge: %w", err)
		}
		// No shale → still a (neutral) status so absence is explicit.
		_ = f.SetStatus(ctx, head, "neutral", "no shale for this PR", card)
		return res, nil
	}

	if err := f.UpsertComment(ctx, pr, render.CommentMarker, card); err != nil {
		return res, fmt.Errorf("post card: %w", err)
	}
	title := fmt.Sprintf("%d session%s of agent evidence", len(shales), plural(len(shales)))
	if len(tamper) > 0 {
		title += " · ⚠️ tamper flags"
	}
	if err := f.SetStatus(ctx, head, "neutral", title, card); err != nil {
		// Status posting is best-effort: the comment is the product.
		fmt.Printf("shale: set status: %v (continuing)\n", err)
	}
	return res, nil
}

// loadShales fetches and parses every .shale/*.yaml at head, returning the
// parseable shales plus tamper warnings (transcript hash mismatch, shale
// edited after its introducing commit). Unparseable files become warnings,
// not failures — hostile input must degrade, not break (D6/D7).
func loadShales(ctx context.Context, f forge.Forge, head string, pr int) ([]*store.Shale, []string) {
	var shales []*store.Shale
	var warnings []string

	paths, err := f.ListDir(ctx, head, ".shale")
	if err != nil {
		warnings = append(warnings, "could not list .shale/: "+err.Error())
		return nil, warnings
	}

	prCommits, _ := f.PRCommits(ctx, pr)
	prSet := map[string]bool{}
	for _, sha := range prCommits {
		prSet[sha] = true
	}

	for _, p := range paths {
		if !strings.HasSuffix(p, ".yaml") {
			continue
		}
		raw, err := f.FileContent(ctx, head, p)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("could not fetch %s", p))
			continue
		}
		s, err := store.Parse(raw)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("invalid shale file %s — ignored", path.Base(p)))
			continue
		}

		// D7a: transcript hash verification.
		if s.Transcript != nil && s.Transcript.Path != "" {
			tp := path.Join(".shale", s.Transcript.Path)
			content, err := f.FileContent(ctx, head, tp)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("transcript missing for session %s", short(s.ID)))
			} else if sha256hex(content) != s.Transcript.SHA256 {
				warnings = append(warnings, fmt.Sprintf("transcript hash mismatch for session %s — treat intent text as unverified", short(s.ID)))
			}
		}

		// D7b: shale edited after its introducing commit (within this PR).
		if len(prSet) > 0 {
			touching, err := f.CommitsTouching(ctx, head, p)
			if err == nil {
				inPR := 0
				for _, sha := range touching {
					if prSet[sha] {
						inPR++
					}
				}
				if inPR > 1 {
					warnings = append(warnings, fmt.Sprintf("shale %s edited after capture — treat its content as unverified", short(s.ID)))
				}
			}
		}

		shales = append(shales, s)
	}
	return shales, warnings
}

func short(id string) string {
	if len(id) > 6 {
		return id[:6]
	}
	return id
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
