// Package fold implements `shale finalize`: the mechanical safety net that
// folds session JSONL event logs into finalized shale YAML files, applying
// redaction, hashing, cost computation, and the git-diff fallback for
// sessions captured without hook adapters (ADR D4 tier 3).
package fold

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/provasign/shale/internal/capture"
	"github.com/provasign/shale/internal/gitx"
	"github.com/provasign/shale/internal/pricing"
	"github.com/provasign/shale/internal/redact"
	"github.com/provasign/shale/internal/store"
)

// Options configures one finalize run.
type Options struct {
	RepoRoot string
	Privacy  string    // full | redacted | hash-only (default redacted)
	Now      time.Time // injection point for tests; zero = time.Now
}

// Result reports what a finalize run produced.
type Result struct {
	Written []string // repo-relative paths created (for auto-commit)
	Skipped []string // session ids skipped (already finalized)
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// Run folds every active session JSONL under .shale/local/ into a finalized
// shale file. Idempotent: folded sessions are archived, and a session whose
// YAML already exists is skipped, never rewritten (spec rule 2).
func Run(opts Options) (Result, error) {
	var res Result
	if opts.Privacy == "" {
		opts.Privacy = store.PrivacyRedacted
	}
	if opts.Now.IsZero() {
		opts.Now = time.Now()
	}
	ids, err := store.ListSessions(opts.RepoRoot)
	if err != nil {
		return res, err
	}
	for _, id := range ids {
		written, err := foldSession(opts, id)
		if err != nil {
			// Fail-open per session: one bad session must not block the push
			// or the other sessions.
			fmt.Fprintf(os.Stderr, "shale finalize: session %s: %v\n", id, err)
			continue
		}
		if written == nil {
			res.Skipped = append(res.Skipped, id)
		} else {
			res.Written = append(res.Written, written...)
		}
		if err := store.ArchiveSession(opts.RepoRoot, id); err != nil {
			fmt.Fprintf(os.Stderr, "shale finalize: archive %s: %v\n", id, err)
		}
	}
	return res, nil
}

// foldSession folds one session. Returns nil paths when the session was
// already finalized (idempotent skip).
func foldSession(opts Options, sessionID string) ([]string, error) {
	yamlRel := filepath.Join(".shale", sessionID+".yaml")
	yamlAbs := filepath.Join(opts.RepoRoot, yamlRel)
	if _, err := os.Stat(yamlAbs); err == nil {
		return nil, nil // append-only: never rewrite
	}

	events, err := store.ReadEvents(store.SessionPath(opts.RepoRoot, sessionID))
	if err != nil {
		return nil, err
	}
	if len(events) == 0 {
		return nil, fmt.Errorf("no events")
	}

	eng := redact.New()
	now := opts.Now.UTC()
	s := &store.Shale{
		ShaleVersion: store.SchemaVersion,
		ID:           sessionID,
		CreatedAt:    events[0].At.UTC(),
		FinalizedAt:  now,
		Privacy:      opts.Privacy,
		Repo: store.Repo{
			RootHint: filepath.Base(opts.RepoRoot),
			Branch:   gitx.Branch(opts.RepoRoot),
		},
		Notes: []store.Note{},
	}

	var prompts []store.Event
	var redactions int
	touches := map[string]*store.FileTouch{}
	var touchOrder []string

	for _, ev := range events {
		switch ev.Kind {
		case store.KindSessionMeta:
			if ev.Tool != "" {
				s.Agent.Tool = ev.Tool
			}
			if ev.ToolVersion != "" {
				s.Agent.ToolVersion = ev.ToolVersion
			}
			if ev.Model != "" {
				s.Agent.Model = ev.Model
			}
			s.Agent.AdapterVersion = capture.AdapterVersion

		case store.KindIntent:
			intent := &store.Intent{
				TitleSHA256: sha256Hex([]byte(ev.Title)), // hash of UNredacted text (proves capture)
				DeclaredAt:  ev.At.UTC(),
			}
			if ev.Body != "" {
				intent.BodySHA256 = sha256Hex([]byte(ev.Body))
			}
			switch opts.Privacy {
			case store.PrivacyHashOnly:
				// Title kept (agent-authored one-liner — the card is useless
				// without it); body reduced to its hash.
				intent.Title, redactions = applyRedact(eng, ev.Title, redactions)
			default:
				intent.Title, redactions = applyRedact(eng, ev.Title, redactions)
				intent.Body, redactions = applyRedact(eng, ev.Body, redactions)
			}
			s.Intent = intent

		case store.KindCompletion:
			c := &store.Completion{
				Model:      ev.Model,
				TokensIn:   ev.TokensIn,
				TokensOut:  ev.TokensOut,
				Iterations: ev.Iterations,
			}
			c.Note, redactions = applyRedact(eng, ev.Note, redactions)
			if c.TokensIn > 0 || c.TokensOut > 0 {
				c.TokensTotal = c.TokensIn + c.TokensOut
			}
			model := c.Model
			if model == "" {
				model = s.Agent.Model
			}
			if usd, ok := pricing.Cost(model, c.TokensIn, c.TokensOut); ok && c.TokensTotal > 0 {
				c.CostUSD = usd
				c.PricingVersion = pricing.Version
			}
			s.Completion = c
			if s.Agent.Model == "" {
				s.Agent.Model = ev.Model
			}

		case store.KindFileTouch:
			path := normalizePath(opts.RepoRoot, ev.Path)
			if path == "" {
				continue
			}
			ft, ok := touches[path]
			if !ok {
				at := ev.At.UTC()
				ft = &store.FileTouch{Path: path, Via: store.ViaHook, FirstTouched: &at}
				touches[path] = ft
				touchOrder = append(touchOrder, path)
			}
			if !contains(ft.Ops, ev.Op) {
				ft.Ops = append(ft.Ops, ev.Op)
			}

		case store.KindCommand:
			cmd, r := eng.Apply(ev.Cmd)
			redactions += r
			s.Commands = append(s.Commands, store.Command{
				Cmd: cmd, ExitCode: ev.ExitCode, At: ev.At.UTC(),
				Classified: ClassifyCommand(ev.Cmd),
			})

		case store.KindPrompt:
			prompts = append(prompts, ev)

		case store.KindNote:
			text, r := eng.Apply(ev.Text)
			redactions += r
			s.Notes = append(s.Notes, store.Note{Text: text, At: ev.At.UTC()})
		}
	}

	if s.Intent != nil {
		s.Intent.PromptCount = len(prompts)
	}

	for _, p := range touchOrder {
		s.Files = append(s.Files, *touches[p])
	}

	// Tier 3 fallback (ADR D4): no hook adapter ran, but the agent declared
	// intent/done — derive the file list from git over the session window.
	if len(s.Files) == 0 && (s.Intent != nil || s.Completion != nil) {
		for _, p := range gitx.FilesChangedSince(opts.RepoRoot, s.CreatedAt) {
			s.Files = append(s.Files, store.FileTouch{
				Path: p, Ops: []string{"edit"}, Via: store.ViaGit,
			})
		}
	}

	written := []string{}

	// Transcript: prompts-only by default (ADR D3a) — user prompts +
	// timestamps, never tool output. Omitted entirely in hash-only mode.
	if opts.Privacy != store.PrivacyHashOnly && len(prompts) > 0 {
		var b strings.Builder
		fmt.Fprintf(&b, "# Session %s — prompts-only transcript\n\n", sessionID)
		for _, p := range prompts {
			text := p.Text
			if opts.Privacy == store.PrivacyRedacted {
				var r int
				text, r = eng.Apply(text)
				redactions += r
			}
			fmt.Fprintf(&b, "**%s**\n\n> %s\n\n", p.At.UTC().Format(time.RFC3339), strings.ReplaceAll(text, "\n", "\n> "))
		}
		content := []byte(b.String())
		transcriptRel := filepath.Join(".shale", "transcripts", sessionID+".md")
		abs := filepath.Join(opts.RepoRoot, transcriptRel)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(abs, content, 0o644); err != nil {
			return nil, err
		}
		s.Transcript = &store.Transcript{
			Path:   filepath.ToSlash(filepath.Join("transcripts", sessionID+".md")),
			SHA256: sha256Hex(content),
			Kind:   "prompts",
		}
		written = append(written, transcriptRel)
	}

	s.Redactions = redactions
	if s.Agent.Tool == "" {
		// Honest provenance (spec rule 4): a session with no session_meta is
		// CLI-only capture; we know nothing about the tool, so omit it.
		s.Agent.AdapterVersion = capture.AdapterVersion
	}

	if err := s.Save(yamlAbs); err != nil {
		return nil, err
	}
	return append(written, yamlRel), nil
}

func applyRedact(eng *redact.Engine, text string, total int) (string, int) {
	out, n := eng.Apply(text)
	return out, total + n
}

// normalizePath converts a hook-supplied path (possibly absolute) into a
// repo-relative slash path. Paths outside the repo are dropped — spec rule 3
// bans absolute paths and a file outside the repo is not PR evidence.
func normalizePath(repoRoot, p string) string {
	if p == "" {
		return ""
	}
	if filepath.IsAbs(p) {
		rel, err := filepath.Rel(repoRoot, p)
		if err != nil || strings.HasPrefix(rel, "..") {
			return ""
		}
		p = rel
	}
	p = filepath.ToSlash(filepath.Clean(p))
	// A unix-rooted path is not IsAbs on Windows (no volume), and a relative
	// path may still climb out of the repo — neither is PR evidence.
	if strings.HasPrefix(p, "/") || p == ".." || strings.HasPrefix(p, "../") {
		return ""
	}
	return p
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

// ClassifyCommand buckets an agent-invoked command for the "checks recorded"
// card section. Heuristic by design; "other" is an honest answer.
func ClassifyCommand(cmd string) string {
	c := strings.ToLower(cmd)
	switch {
	case containsAny(c, "gitleaks", "semgrep", "trivy", "snyk", "govulncheck", "bandit", "grype"):
		return "scan"
	case containsAny(c, "golangci-lint", "eslint", "ruff", "flake8", "pylint", "rubocop", "clippy", "go vet", "staticcheck", "tflint"):
		return "lint"
	case containsAny(c, "go test", "pytest", "npm test", "yarn test", "pnpm test", "jest", "vitest", "cargo test", "rspec", "mvn test", "gradle test", "make test"):
		return "test"
	case containsAny(c, "go build", "npm run build", "yarn build", "cargo build", "make build", "mvn package", "gradle build", "docker build", "tsc"):
		return "build"
	default:
		return "other"
	}
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
