// Package fold implements `shale finalize`: the mechanical safety net that
// folds session JSONL event logs into finalized shale YAML files, applying
// redaction, hashing, cost computation, and the git-diff fallback for paths
// not captured by hook adapters (ADR D4 tier 3).
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
	"github.com/provasign/shale/internal/commandfmt"
	"github.com/provasign/shale/internal/gitx"
	"github.com/provasign/shale/internal/pricing"
	"github.com/provasign/shale/internal/redact"
	"github.com/provasign/shale/internal/store"
)

// transcriptsEnabled is a build-time feature flag, deliberately NOT exposed
// through config.yaml, flags, or env: while false, finalize never writes a
// prompt transcript into the repo, regardless of privacy mode. Raw prompts
// remain in .shale/local/ on the laptop only (gitignored), and prompt_count /
// the intent title hash still work, so flipping this later changes nothing
// about the evidence schema. Off until (a) the redaction layer has earned
// confidence beyond pattern matching and (b) the regulatory questions around
// persisting developers' raw prompts are settled. See README "Privacy".
const transcriptsEnabled = false

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
// YAML already exists is never rewritten (spec rule 2) — post-finalize task
// evidence folds into a continuation file instead.
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
// already finalized and the remnant log carries no task evidence (idempotent
// skip).
func foldSession(opts Options, sessionID string) ([]string, error) {
	events, err := store.ReadEvents(store.SessionPath(opts.RepoRoot, sessionID))
	if err != nil {
		return nil, err
	}
	if len(events) == 0 {
		return nil, fmt.Errorf("no events")
	}

	evidenceID := sessionID
	yamlRel := filepath.Join(".shale", sessionID+".yaml")
	continuation := false
	if _, err := os.Stat(filepath.Join(opts.RepoRoot, yamlRel)); err == nil {
		// Already finalized — append-only, never rewrite (spec rule 2). But
		// hook adapters recreate the JSONL when events land after a finalize
		// (e.g. a `shale done` that arrived after the pre-push hook ran).
		// Real task evidence in that remnant folds into a continuation file;
		// ambient capture alone (commands, prompts) is archived without
		// minting one.
		if !hasTaskEvidence(events) {
			return nil, nil
		}
		continuation = true
		for n := 2; ; n++ {
			evidenceID = fmt.Sprintf("%s-c%d", sessionID, n)
			yamlRel = filepath.Join(".shale", evidenceID+".yaml")
			if _, err := os.Stat(filepath.Join(opts.RepoRoot, yamlRel)); os.IsNotExist(err) {
				break
			}
		}
	}
	yamlAbs := filepath.Join(opts.RepoRoot, yamlRel)

	eng := redact.New()
	now := opts.Now.UTC()
	s := &store.Shale{
		ShaleVersion: store.SchemaVersion,
		ID:           evidenceID,
		CreatedAt:    events[0].At.UTC(),
		FinalizedAt:  now,
		Privacy:      opts.Privacy,
		Repo: store.Repo{
			RootHint: filepath.Base(opts.RepoRoot),
			Branch:   gitx.Branch(opts.RepoRoot),
		},
		Notes: []store.Note{},
	}
	if continuation {
		s.Notes = append(s.Notes, store.Note{
			Text: fmt.Sprintf("continuation of session %s — events captured after its evidence was finalized", sessionID),
			At:   s.CreatedAt,
		})
	}

	var prompts []store.Event
	var redactions int
	touches := map[string]*store.FileTouch{}
	var touchOrder []string

	// A session can hold several intent→done arcs (one per task). A
	// completion always belongs to the intent that precedes it — pairing it
	// with an intent declared later would attribute one task's outcome to
	// another.
	type taskCycle struct {
		intent      *store.Intent
		completion  *store.Completion
		completedAt time.Time
	}
	var cycles []taskCycle
	var cur taskCycle

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
			// A new intent never adopts an earlier cycle's completion.
			if cur.intent != nil || cur.completion != nil {
				cycles = append(cycles, cur)
				cur = taskCycle{}
			}
			cur.intent = intent

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
			if cur.completion != nil {
				cycles = append(cycles, cur)
				cur = taskCycle{}
			}
			cur.completion = c
			cur.completedAt = ev.At.UTC()
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
			rawCmd := commandfmt.ForEvidence(opts.RepoRoot, ev.Cmd)
			if rawCmd == "" {
				continue
			}
			cmd, r := eng.Apply(rawCmd)
			redactions += r
			s.Commands = append(s.Commands, store.Command{
				Cmd: cmd, ExitCode: ev.ExitCode, At: ev.At.UTC(),
				Classified: ClassifyCommand(rawCmd),
			})

		case store.KindPrompt:
			prompts = append(prompts, ev)

		case store.KindNote:
			text, r := eng.Apply(ev.Text)
			redactions += r
			s.Notes = append(s.Notes, store.Note{Text: text, At: ev.At.UTC()})
		}
	}

	if cur.intent != nil || cur.completion != nil {
		cycles = append(cycles, cur)
	}
	if n := len(cycles); n > 0 {
		// The last cycle is the session's current state; earlier cycles stay
		// in the evidence as notes — never dropped, never cross-paired.
		s.Intent = cycles[n-1].intent
		s.Completion = cycles[n-1].completion
		for _, c := range cycles[:n-1] {
			if c.intent != nil {
				s.Notes = append(s.Notes, store.Note{
					Text: "earlier task in this session — intent: " + c.intent.Title,
					At:   c.intent.DeclaredAt,
				})
			}
			if c.completion != nil {
				text := "earlier task in this session — completed"
				if c.completion.Note != "" {
					text += ": " + c.completion.Note
				}
				s.Notes = append(s.Notes, store.Note{Text: text, At: c.completedAt})
			}
		}
	}

	if s.Intent != nil {
		s.Intent.PromptCount = len(prompts)
	}

	// Hook-reported paths the repo itself refuses to track are not PR
	// evidence: gitignored files (.env, credentials, key material) are
	// exactly where secrets live, so they are dropped here. The tier-3 git
	// fallback below never resurfaces them — git log/status do not report
	// ignored files.
	ignored := gitx.IgnoredPaths(opts.RepoRoot, touchOrder)
	for _, p := range touchOrder {
		if ignored[p] {
			continue
		}
		s.Files = append(s.Files, *touches[p])
	}

	// Tier 3 fallback (ADR D4): if the agent declared intent/done, derive
	// missing paths from git over the session window. Hook evidence wins for
	// paths it saw; git fills only the gaps, with ops derived from git's own
	// statuses — a deletion recorded as "edit" is misevidence.
	if s.Intent != nil || s.Completion != nil {
		for _, fc := range gitx.FilesChangedSince(opts.RepoRoot, s.CreatedAt) {
			if touches[fc.Path] != nil {
				continue
			}
			s.Files = append(s.Files, store.FileTouch{
				Path: fc.Path, Ops: fc.Ops, Via: store.ViaGit,
			})
		}
	}

	written := []string{}

	// Transcript: prompts-only by default (ADR D3a) — user prompts +
	// timestamps, never tool output. Omitted entirely in hash-only mode.
	if transcriptsEnabled && opts.Privacy != store.PrivacyHashOnly && len(prompts) > 0 {
		var b strings.Builder
		fmt.Fprintf(&b, "# Session %s — prompts-only transcript\n\n", evidenceID)
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
		transcriptRel := filepath.Join(".shale", "transcripts", evidenceID+".md")
		abs := filepath.Join(opts.RepoRoot, transcriptRel)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(abs, content, 0o644); err != nil {
			return nil, err
		}
		s.Transcript = &store.Transcript{
			Path:   filepath.ToSlash(filepath.Join("transcripts", evidenceID+".md")),
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

// hasTaskEvidence reports whether an event log carries anything worth a
// continuation evidence file: an agent-declared intent or completion, a file
// touch, or a manual note. Ambient capture (commands, prompts, session
// markers) alone does not.
func hasTaskEvidence(events []store.Event) bool {
	for _, ev := range events {
		switch ev.Kind {
		case store.KindIntent, store.KindCompletion, store.KindFileTouch, store.KindNote:
			return true
		}
	}
	return false
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
	return commandfmt.Classify(cmd)
}
