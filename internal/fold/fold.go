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
	// Sessions, when set, are the only sessions folded: shale done folds
	// its own agent's session, never the others open in the checkout.
	Sessions []string
	// ClosedOnly folds only the tasks that are closed (shale done, or the
	// agent's session ended), for a commit to carry their evidence; a task
	// still in progress stays in the log, to be folded once closed.
	ClosedOnly bool
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
		if len(opts.Sessions) > 0 && !contains(opts.Sessions, id) {
			continue
		}
		written, rest, err := foldSession(opts, id)
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
		var aerr error
		switch {
		case rest == nil:
			aerr = store.ArchiveSession(opts.RepoRoot, id)
		case len(rest.folded) > 0:
			aerr = store.ArchivePart(opts.RepoRoot, id, rest.folded, rest.open)
		}
		if aerr != nil {
			fmt.Fprintf(os.Stderr, "shale finalize: archive %s: %v\n", id, aerr)
		}
	}
	return res, nil
}

// foldSession folds one session: one evidence file per task. A completion
// (shale done) closes a task, and a request declared after it starts the
// next one, in the same session: an agent working through several features
// in one conversation used to get one file, holding only the last request,
// the earlier ones reduced to notes; a requirement checked against the
// requests (Stele) then read the next feature as an edit of the one before
// (a developer trial, 2026-11-03). Returns nil paths when the session was
// already finalized and the remnant log carries no task evidence
// (idempotent skip).
func foldSession(opts Options, sessionID string) ([]string, *remainder, error) {
	events, err := store.ReadEvents(store.SessionPath(opts.RepoRoot, sessionID))
	if err != nil {
		return nil, nil, err
	}
	if len(events) == 0 {
		return nil, nil, fmt.Errorf("no events")
	}
	var rest *remainder
	if opts.ClosedOnly {
		// Fold the closed tasks only; the one in progress stays open.
		starts := taskStarts(events)
		last := starts[len(starts)-1]
		if !closesTask(events[last:]) {
			rest = &remainder{folded: events[:last], open: withMeta(events, last)}
			if last == 0 {
				return []string{}, rest, nil // nothing closed yet
			}
			events = events[:last]
		}
	}
	exists := func(id string) bool {
		_, err := os.Stat(filepath.Join(opts.RepoRoot, ".shale", id+".yaml"))
		return err == nil
	}
	continuation := exists(sessionID)
	if continuation && !hasTaskEvidence(opts.RepoRoot, events) {
		// Already finalized — append-only, never rewrite (spec rule 2). Ambient
		// capture alone (commands, prompts) is archived without minting a file.
		return nil, rest, nil
	}
	tasks := splitTasks(events)
	written := []string{}
	for i, task := range tasks {
		id, note := sessionID, ""
		if continuation || i > 0 {
			// Hook adapters recreate the JSONL when events land after a
			// finalize (e.g. a shale done that arrived after the pre-push
			// hook ran); a later task of the session is another file too.
			for n := 2; ; n++ {
				if id = fmt.Sprintf("%s-c%d", sessionID, n); !exists(id) {
					break
				}
			}
			note = fmt.Sprintf("continuation of session %s — events captured after its evidence was finalized", sessionID)
			if i > 0 {
				note = fmt.Sprintf("a later task of session %s", sessionID)
			}
		}
		var until time.Time
		if i+1 < len(tasks) {
			until = tasks[i+1][0].At // the next task's files are its own
		}
		w, err := foldRecord(opts, id, task, note, until)
		if err != nil {
			return written, nil, err
		}
		written = append(written, w...)
	}
	return written, rest, nil
}

// remainder is what a closed-only fold leaves: the events it folded, to
// archive, and the open task's, which stay the log.
type remainder struct{ folded, open []store.Event }

// taskStarts are the indexes where the session's tasks begin: after a
// closing event (a completion, or the session closing), a request declared
// later starts the next task.
func taskStarts(events []store.Event) []int {
	starts := []int{0}
	closed := -1
	for i, ev := range events {
		switch ev.Kind {
		case store.KindCompletion:
			closed = i + 1
		case store.KindSessionClose:
			if !store.Resumed(events, i) {
				closed = i + 1
			}
		case store.KindIntent:
			if closed > starts[len(starts)-1] {
				starts = append(starts, closed)
				closed = -1
			}
		}
	}
	return starts
}

// closesTask reports whether a task's events close it.
func closesTask(events []store.Event) bool {
	for i, ev := range events {
		if ev.Kind == store.KindCompletion || ev.Kind == store.KindSessionClose && !store.Resumed(events, i) {
			return true
		}
	}
	return false
}

// withMeta is the events from start on, after the session's metadata when
// start is not the beginning.
func withMeta(events []store.Event, start int) []store.Event {
	if start == 0 {
		return events
	}
	var out []store.Event
	for _, ev := range events[:start] {
		if ev.Kind == store.KindSessionMeta {
			out = append(out, ev)
		}
	}
	return append(out, events[start:]...)
}

// splitTasks cuts a session's events into tasks (taskStarts): after a
// closing event, a request declared later starts the next task, with the
// prompts and commands that came between. A request declared again before
// the completion restates the same task. Later tasks get the session's
// metadata.
func splitTasks(events []store.Event) [][]store.Event {
	starts := taskStarts(events)
	var tasks [][]store.Event
	for i, start := range starts {
		end := len(events)
		if i+1 < len(starts) {
			end = starts[i+1]
		}
		tasks = append(tasks, withMeta(events[:end], start))
	}
	return tasks
}

// foldRecord writes one task's evidence file, evidenceID, from its events.
// note, when set, explains how the file relates to its session; until, when
// set, bounds the git fallback to commits made before the next task.
func foldRecord(opts Options, evidenceID string, events []store.Event, note string, until time.Time) ([]string, error) {
	yamlRel := filepath.Join(".shale", evidenceID+".yaml")
	yamlAbs := filepath.Join(opts.RepoRoot, yamlRel)

	// A project's own private terms (redact.TermFiles) are masked with the
	// secrets; a term list that cannot be read stops the record from being
	// written at all.
	eng, err := redact.ForRepo(opts.RepoRoot)
	if err != nil {
		return nil, err
	}
	now := opts.Now.UTC()
	s := &store.Shale{
		ShaleVersion: store.SchemaVersion,
		ID:           evidenceID,
		CreatedAt:    events[0].At.UTC(),
		FinalizedAt:  now,
		Privacy:      opts.Privacy,
		Repo: store.Repo{
			RootHint: filepath.Base(opts.RepoRoot),
			Branch:   eng.Clean(gitx.Branch(opts.RepoRoot)),
		},
		Notes: []store.Note{},
	}
	if note != "" {
		s.Notes = append(s.Notes, store.Note{Text: note, At: s.CreatedAt})
	}

	var prompts []store.Event
	var redactions int
	ownNotes := 0 // notes from shale note, as opposed to the ones folding writes
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

	for i, ev := range events {
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
				For:         ev.For, // an identifier the agent chose, never prose
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
			// A new intent never adopts an earlier cycle's completion; one
			// declared again before the completion restates the request.
			switch {
			case cur.completion != nil:
				cycles = append(cycles, cur)
				cur = taskCycle{}
			case cur.intent != nil:
				s.Notes = append(s.Notes, store.Note{Text: "request restated; it read: " + cur.intent.Title, At: cur.intent.DeclaredAt})
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

		case store.KindSessionClose:
			// The session ended for good with its task never reported
			// done: the task is over, and says how it ended.
			if cur.intent != nil && cur.completion == nil && !store.Resumed(events, i) {
				cur.completion = &store.Completion{Note: "the agent's session ended without shale done"}
				cur.completedAt = ev.At.UTC()
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
			ownNotes++
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
		ft := *touches[p]
		var r int
		ft.Path, r = eng.Apply(ft.Path)
		redactions += r
		s.Files = append(s.Files, ft)
	}

	// Tier 3 fallback (ADR D4): if the agent declared intent/done, derive
	// missing paths from git over the session window. Hook evidence wins for
	// paths it saw; git fills only the gaps, with ops derived from git's own
	// statuses — a deletion recorded as "edit" is misevidence.
	if s.Intent != nil || s.Completion != nil {
		for _, fc := range gitx.FilesChangedBetween(opts.RepoRoot, s.CreatedAt, until) {
			if touches[fc.Path] != nil {
				continue
			}
			path, r := eng.Apply(fc.Path)
			redactions += r
			s.Files = append(s.Files, store.FileTouch{
				Path: path, Ops: fc.Ops, Via: store.ViaGit,
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

	// A record of nothing but ambient capture (commands, prompts) says
	// nothing about work in this repository: no request, no completion, no
	// file here, no note. It is not written, so it is never published: an
	// agent working elsewhere from this checkout (a trial in another
	// folder) had its commands committed and pushed with someone else's
	// work (2026-11-03). Its events stay in the local archive.
	if s.Intent == nil && s.Completion == nil && len(s.Files) == 0 && ownNotes == 0 {
		return nil, nil
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
func hasTaskEvidence(repoRoot string, events []store.Event) bool {
	for _, ev := range events {
		switch ev.Kind {
		case store.KindIntent, store.KindCompletion, store.KindNote:
			return true
		case store.KindFileTouch:
			// A file outside this repository is no work done here.
			if normalizePath(repoRoot, ev.Path) != "" {
				return true
			}
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
