package fold

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/provasign/shale/internal/store"
)

var (
	t0  = time.Date(2026, 6, 9, 14, 2, 11, 0, time.UTC)
	now = time.Date(2026, 6, 9, 14, 41, 3, 0, time.UTC)
)

func intPtr(i int) *int { return &i }

func initRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, args := range [][]string{
		{"init", "-b", "main"},
		{"config", "user.email", "t@e.com"},
		{"config", "user.name", "T"},
		{"config", "commit.gpgsign", "false"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	// Resolve symlinks (macOS /tmp → /private/tmp) so normalizePath agrees
	// with what git reports.
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func seedFullSession(t *testing.T, root, id string) {
	t.Helper()
	events := []store.Event{
		{Kind: store.KindSessionMeta, At: t0, Tool: "claude-code", Model: "claude-fable-5", SessionID: id},
		{Kind: store.KindPrompt, At: t0, Text: "Add rate limiting, token is ghp_aBcDeFgHiJkLmNoPqRsTuVwXyZ0123456789 oops"},
		{Kind: store.KindIntent, At: t0.Add(time.Second), Title: "Add rate limiting to the login endpoint", Body: "Brute force in prod. Redis, 10/min."},
		{Kind: store.KindFileTouch, At: t0.Add(3 * time.Minute), Path: "internal/auth/ratelimit.go", Op: "write"},
		{Kind: store.KindFileTouch, At: t0.Add(4 * time.Minute), Path: "internal/auth/ratelimit.go", Op: "edit"},
		{Kind: store.KindFileTouch, At: t0.Add(5 * time.Minute), Path: filepath.Join(root, "internal/auth/login.go"), Op: "edit"},
		{Kind: store.KindCommand, At: t0.Add(30 * time.Minute), Cmd: "go test ./internal/auth/...", ExitCode: intPtr(0)},
		{Kind: store.KindCommand, At: t0.Add(31 * time.Minute), Cmd: "gitleaks detect --no-banner", ExitCode: intPtr(0)},
		{Kind: store.KindCompletion, At: t0.Add(38 * time.Minute), Note: "Limiter done.", Model: "claude-fable-5", TokensIn: 32000, TokensOut: 15000, Iterations: 3},
		{Kind: store.KindSessionEnd, At: t0.Add(39 * time.Minute)},
	}
	for _, ev := range events {
		if err := store.AppendEvent(root, id, ev); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRunFullSession(t *testing.T) {
	root := initRepo(t)
	seedFullSession(t, root, "SESSFULL01")

	res, err := Run(Options{RepoRoot: root, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Written) != 1 { // yaml only — transcripts are feature-flagged off
		t.Fatalf("written = %v", res.Written)
	}

	s, err := store.Load(filepath.Join(root, ".shale", "SESSFULL01.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if s.Agent.Tool != "claude-code" || s.Agent.Model != "claude-fable-5" {
		t.Fatalf("agent = %+v", s.Agent)
	}
	if s.Intent == nil || s.Intent.Title != "Add rate limiting to the login endpoint" {
		t.Fatalf("intent = %+v", s.Intent)
	}
	wantHash := sha256.Sum256([]byte("Add rate limiting to the login endpoint"))
	if s.Intent.TitleSHA256 != hex.EncodeToString(wantHash[:]) {
		t.Fatal("title hash is not sha256 of unredacted title")
	}
	if s.Intent.PromptCount != 1 {
		t.Fatalf("prompt_count = %d", s.Intent.PromptCount)
	}
	if s.Completion == nil || s.Completion.TokensTotal != 47000 {
		t.Fatalf("completion = %+v", s.Completion)
	}
	if s.Completion.CostUSD != 0.54 || s.Completion.PricingVersion == "" {
		t.Fatalf("cost = %v pricing_version = %q", s.Completion.CostUSD, s.Completion.PricingVersion)
	}
	if len(s.Files) != 2 {
		t.Fatalf("files = %+v", s.Files)
	}
	if s.Files[0].Path != "internal/auth/ratelimit.go" || s.Files[0].Via != store.ViaHook {
		t.Fatalf("file[0] = %+v", s.Files[0])
	}
	if len(s.Files[0].Ops) != 2 { // write + edit deduped into one entry
		t.Fatalf("ops = %v", s.Files[0].Ops)
	}
	if s.Files[1].Path != "internal/auth/login.go" {
		t.Fatalf("absolute path not normalized: %+v", s.Files[1])
	}
	if s.Commands[0].Classified != "test" || s.Commands[1].Classified != "scan" {
		t.Fatalf("classification = %+v", s.Commands)
	}

	// Transcripts are feature-flagged OFF (transcriptsEnabled = false): the
	// raw prompt — which carries a seeded secret — must never reach the repo,
	// in any privacy mode. prompt_count and the title hash still work.
	if s.Transcript != nil {
		t.Fatalf("transcript written despite the feature flag: %+v", s.Transcript)
	}
	if _, err := os.Stat(filepath.Join(root, ".shale", "transcripts")); !os.IsNotExist(err) {
		t.Fatal("transcripts directory created despite the feature flag")
	}
	// The seeded secret must not appear anywhere in the committed YAML either.
	rawYAML, err := os.ReadFile(filepath.Join(root, ".shale", "SESSFULL01.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(rawYAML), "ghp_") {
		t.Fatal("secret survived into committed evidence")
	}

	// Idempotency: re-run with no new events is a clean no-op.
	res2, err := Run(Options{RepoRoot: root, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if len(res2.Written) != 0 {
		t.Fatalf("second run wrote %v", res2.Written)
	}
}

func TestRunGitFallbackWhenNoHooks(t *testing.T) {
	root := initRepo(t)
	// CLI-only session: intent + done, zero hook events (e.g. Copilot).
	for _, ev := range []store.Event{
		{Kind: store.KindIntent, At: time.Now().UTC().Add(-time.Minute), Title: "Fix the flaky test"},
		{Kind: store.KindCompletion, At: time.Now().UTC(), Note: "fixed"},
	} {
		if err := store.AppendEvent(root, "SESSCLI01", ev); err != nil {
			t.Fatal(err)
		}
	}
	os.WriteFile(filepath.Join(root, "flaky_test.go"), []byte("package x"), 0o644)

	if _, err := Run(Options{RepoRoot: root, Now: time.Now()}); err != nil {
		t.Fatal(err)
	}
	s, err := store.Load(filepath.Join(root, ".shale", "SESSCLI01.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Files) != 1 || s.Files[0].Path != "flaky_test.go" || s.Files[0].Via != store.ViaGit {
		t.Fatalf("git fallback files = %+v", s.Files)
	}
	if len(s.Files[0].Ops) != 1 || s.Files[0].Ops[0] != "write" {
		t.Fatalf("untracked file ops = %v, want [write]", s.Files[0].Ops)
	}
	if s.Files[0].FirstTouched != nil {
		t.Fatal("via:git entries must omit first_touched (we don't know it)")
	}
}

func TestRunGitFallbackFillsMissingHookPaths(t *testing.T) {
	root := initRepo(t)
	sessionStart := time.Now().UTC().Add(-time.Minute)
	for _, ev := range []store.Event{
		{Kind: store.KindIntent, At: sessionStart, Title: "Update login behavior"},
		{Kind: store.KindFileTouch, At: sessionStart.Add(time.Second), Path: "internal/auth/auth.go", Op: "edit"},
		{Kind: store.KindCompletion, At: sessionStart.Add(30 * time.Second), Note: "updated"},
	} {
		if err := store.AppendEvent(root, "SESSMIXED01", ev); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(root, "internal/auth"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "internal/auth/auth.go"), []byte("package auth"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"add", "--", "internal/auth/auth.go", "main.go"},
		{"commit", "-m", "update login behavior"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	if _, err := Run(Options{RepoRoot: root, Now: time.Now()}); err != nil {
		t.Fatal(err)
	}
	s, err := store.Load(filepath.Join(root, ".shale", "SESSMIXED01.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Files) != 2 {
		t.Fatalf("files = %+v", s.Files)
	}
	if s.Files[0].Path != "internal/auth/auth.go" || s.Files[0].Via != store.ViaHook {
		t.Fatalf("hook file = %+v", s.Files[0])
	}
	if s.Files[1].Path != "main.go" || s.Files[1].Via != store.ViaGit {
		t.Fatalf("git fallback file = %+v", s.Files[1])
	}
	if len(s.Files[1].Ops) != 1 || s.Files[1].Ops[0] != "write" {
		t.Fatalf("newly added file ops = %v, want [write]", s.Files[1].Ops)
	}
}

func TestRunCleansCommandEvidenceBeforePersistence(t *testing.T) {
	root := initRepo(t)
	rawCmd := `cat > pkg/query_test.go <<'EOF'
package pkg

const token = "ghp_aBcDeFgHiJkLmNoPqRsTuVwXyZ0123456789"
EOF
cd ` + root + `; go test ./pkg/criteria -run TestTranslator_NumericPropertyComparison -v 2>&1 | grep -E "PASS|FAIL" | head -25`
	for _, ev := range []store.Event{
		{Kind: store.KindIntent, At: t0, Title: "Fix query parsing"},
		{Kind: store.KindCommand, At: t0.Add(time.Minute), Cmd: rawCmd, ExitCode: intPtr(0)},
		{Kind: store.KindCompletion, At: t0.Add(2 * time.Minute), Note: "fixed"},
	} {
		if err := store.AppendEvent(root, "SESSCMD01", ev); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := Run(Options{RepoRoot: root, Now: now}); err != nil {
		t.Fatal(err)
	}
	s, err := store.Load(filepath.Join(root, ".shale", "SESSCMD01.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Commands) != 1 {
		t.Fatalf("commands = %+v", s.Commands)
	}
	got := s.Commands[0].Cmd
	want := "go test ./pkg/criteria -run TestTranslator_NumericPropertyComparison -v 2>&1"
	if got != want || s.Commands[0].Classified != "test" {
		t.Fatalf("command = %#v, want %q classified as test", s.Commands[0], want)
	}
	rawYAML, err := os.ReadFile(filepath.Join(root, ".shale", "SESSCMD01.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{"ghp_", "package pkg", root, "/Users/", "grep -E", "head -25"} {
		if strings.Contains(string(rawYAML), banned) {
			t.Fatalf("committed command evidence leaked %q:\n%s", banned, rawYAML)
		}
	}
}

// Regression: a session with two intent→done arcs must never pair one task's
// completion with another task's intent (found live: session d38090f4 in the
// prism repo published the second intent with the first completion note).
func TestRunPairsCompletionWithPrecedingIntent(t *testing.T) {
	root := initRepo(t)
	for _, ev := range []store.Event{
		{Kind: store.KindIntent, At: t0, Title: "Commit the init output"},
		{Kind: store.KindCompletion, At: t0.Add(time.Minute), Note: "init committed"},
		{Kind: store.KindIntent, At: t0.Add(2 * time.Minute), Title: "Untrack the paused tool"},
	} {
		if err := store.AppendEvent(root, "SESSTWOTASK01", ev); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := Run(Options{RepoRoot: root, Now: now}); err != nil {
		t.Fatal(err)
	}
	// One file per task: the first keeps its own request and completion,
	// the second is a file of its own and adopts no completion.
	first, err := store.Load(filepath.Join(root, ".shale", "SESSTWOTASK01.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if first.Intent == nil || first.Intent.Title != "Commit the init output" || first.Completion == nil || first.Completion.Note != "init committed" {
		t.Fatalf("first task = %+v / %+v", first.Intent, first.Completion)
	}
	second, err := store.Load(filepath.Join(root, ".shale", "SESSTWOTASK01-c2.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if second.Intent == nil || second.Intent.Title != "Untrack the paused tool" {
		t.Fatalf("second task intent = %+v", second.Intent)
	}
	if second.Completion != nil {
		t.Fatalf("completion = %+v — the first task's completion must not attach to the second intent", second.Completion)
	}
}

// A request declared again before its completion restates the same task:
// one file, the latest wording, the earlier one in a note.
func TestRunRestatedRequestIsOneTask(t *testing.T) {
	root := initRepo(t)
	for _, ev := range []store.Event{
		{Kind: store.KindIntent, At: t0, Title: "Limit loans to 5 books"},
		{Kind: store.KindIntent, At: t0.Add(time.Minute), Title: "Limit loans to 5 books, 10 in summer"},
		{Kind: store.KindCompletion, At: t0.Add(2 * time.Minute), Note: "limit in place"},
	} {
		if err := store.AppendEvent(root, "SESSRESTATE01", ev); err != nil {
			t.Fatal(err)
		}
	}
	res, err := Run(Options{RepoRoot: root, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Written) != 1 {
		t.Fatalf("written = %v, want one file", res.Written)
	}
	s, err := store.Load(filepath.Join(root, ".shale", "SESSRESTATE01.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if s.Intent == nil || s.Intent.Title != "Limit loans to 5 books, 10 in summer" || s.Completion == nil {
		t.Fatalf("intent %+v completion %+v", s.Intent, s.Completion)
	}
	restated := false
	for _, n := range s.Notes {
		restated = restated || strings.Contains(n.Text, "restated") && strings.Contains(n.Text, "Limit loans to 5 books")
	}
	if !restated {
		t.Fatalf("earlier wording not in the notes: %+v", s.Notes)
	}
}

// Regression: events captured after a session was finalized (e.g. `shale
// done` landing after the pre-push hook ran) must fold into a continuation
// file, and re-archiving the same session id must append, not clobber.
func TestRunContinuationAfterFinalize(t *testing.T) {
	root := initRepo(t)
	seedFullSession(t, root, "SESSCONT01")
	if _, err := Run(Options{RepoRoot: root, Now: now}); err != nil {
		t.Fatal(err)
	}

	// Ambient-only remnant: no continuation file, archive appended.
	for _, ev := range []store.Event{
		{Kind: store.KindCommand, At: now.Add(time.Minute), Cmd: "git push origin main"},
	} {
		if err := store.AppendEvent(root, "SESSCONT01", ev); err != nil {
			t.Fatal(err)
		}
	}
	res, err := Run(Options{RepoRoot: root, Now: now.Add(2 * time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Written) != 0 {
		t.Fatalf("ambient remnant minted evidence: %v", res.Written)
	}

	// Remnant with a completion: continuation file carries it.
	for _, ev := range []store.Event{
		{Kind: store.KindCompletion, At: now.Add(3 * time.Minute), Note: "closing note that raced the push"},
	} {
		if err := store.AppendEvent(root, "SESSCONT01", ev); err != nil {
			t.Fatal(err)
		}
	}
	res, err = Run(Options{RepoRoot: root, Now: now.Add(4 * time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Written) != 1 || !strings.HasSuffix(res.Written[0], "SESSCONT01-c2.yaml") {
		t.Fatalf("written = %v, want the -c2 continuation", res.Written)
	}
	s, err := store.Load(filepath.Join(root, ".shale", "SESSCONT01-c2.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if s.ID != "SESSCONT01-c2" {
		t.Fatalf("continuation id = %q", s.ID)
	}
	if s.Completion == nil || s.Completion.Note != "closing note that raced the push" {
		t.Fatalf("continuation completion = %+v", s.Completion)
	}
	var sawContinuationNote bool
	for _, n := range s.Notes {
		if strings.Contains(n.Text, "continuation of session SESSCONT01") {
			sawContinuationNote = true
		}
	}
	if !sawContinuationNote {
		t.Fatalf("continuation provenance note missing: %+v", s.Notes)
	}

	// The original evidence file was never rewritten (spec rule 2)...
	orig, err := store.Load(filepath.Join(root, ".shale", "SESSCONT01.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if orig.Completion == nil || orig.Completion.Note != "Limiter done." {
		t.Fatalf("original evidence changed: %+v", orig.Completion)
	}
	// ...and the archive holds ALL events for the id — appended, not clobbered.
	raw, err := os.ReadFile(filepath.Join(root, ".shale", "local", "archive", "SESSCONT01.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	for _, must := range []string{"Add rate limiting", "git push origin main", "closing note that raced the push"} {
		if !strings.Contains(string(raw), must) {
			t.Fatalf("archive lost %q — clobbered instead of appended", must)
		}
	}
}

func TestRunDropsGitIgnoredHookPaths(t *testing.T) {
	root := initRepo(t)
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte(".env\nsecrets/\n*.pem\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, ev := range []store.Event{
		{Kind: store.KindIntent, At: t0, Title: "Wire up the payment client"},
		{Kind: store.KindFileTouch, At: t0.Add(time.Minute), Path: "internal/pay/client.go", Op: "write"},
		{Kind: store.KindFileTouch, At: t0.Add(2 * time.Minute), Path: ".env", Op: "write"},
		{Kind: store.KindFileTouch, At: t0.Add(3 * time.Minute), Path: "secrets/stripe.json", Op: "write"},
		{Kind: store.KindFileTouch, At: t0.Add(4 * time.Minute), Path: filepath.Join(root, "server.pem"), Op: "write"},
		{Kind: store.KindCompletion, At: t0.Add(5 * time.Minute), Note: "done"},
	} {
		if err := store.AppendEvent(root, "SESSIGN01", ev); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := Run(Options{RepoRoot: root, Now: now}); err != nil {
		t.Fatal(err)
	}
	s, err := store.Load(filepath.Join(root, ".shale", "SESSIGN01.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range s.Files {
		switch f.Path {
		case ".env", "secrets/stripe.json", "server.pem":
			t.Fatalf("gitignored path survived into evidence: %+v", f)
		}
	}
	var sawClient bool
	for _, f := range s.Files {
		if f.Path == "internal/pay/client.go" && f.Via == store.ViaHook {
			sawClient = true
		}
	}
	if !sawClient {
		t.Fatalf("non-ignored hook path missing: %+v", s.Files)
	}
}

func TestRunHashOnlyPrivacy(t *testing.T) {
	root := initRepo(t)
	seedFullSession(t, root, "SESSHASH01")
	if _, err := Run(Options{RepoRoot: root, Privacy: store.PrivacyHashOnly, Now: now}); err != nil {
		t.Fatal(err)
	}
	s, err := store.Load(filepath.Join(root, ".shale", "SESSHASH01.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if s.Transcript != nil {
		t.Fatal("hash-only must not write a transcript")
	}
	if s.Intent.Body != "" {
		t.Fatal("hash-only must drop intent body text")
	}
	if s.Intent.BodySHA256 == "" || s.Intent.Title == "" {
		t.Fatal("hash-only keeps body hash and title")
	}
	if _, err := os.Stat(filepath.Join(root, ".shale", "transcripts", "SESSHASH01.md")); !os.IsNotExist(err) {
		t.Fatal("transcript file written in hash-only mode")
	}
}

func TestClassifyCommand(t *testing.T) {
	cases := map[string]string{
		"go test ./...":           "test",
		"npx jest --ci":           "test",
		"golangci-lint run":       "lint",
		"ruff check .":            "lint",
		"gitleaks detect":         "scan",
		"semgrep scan --config p": "scan",
		"go build ./...":          "build",
		"docker build -t x .":     "build",
		"ls -la":                  "other",
	}
	for cmd, want := range cases {
		if got := ClassifyCommand(cmd); got != want {
			t.Errorf("ClassifyCommand(%q) = %q, want %q", cmd, got, want)
		}
	}
}

func TestNormalizePathRejectsOutsideRepo(t *testing.T) {
	repo := t.TempDir() // platform-real absolute root, valid on Windows too
	if got := normalizePath(repo, filepath.Join(repo, "..", "passwd")); got != "" {
		t.Fatalf("path outside repo accepted: %q", got)
	}
	if got := normalizePath(repo, "/etc/passwd"); got != "" {
		t.Fatalf("rooted path accepted: %q", got)
	}
	if got := normalizePath(repo, "../escape.go"); got != "" {
		t.Fatalf("relative path escaping the repo accepted: %q", got)
	}
	if got := normalizePath(repo, filepath.Join(repo, "a", "b.go")); got != "a/b.go" {
		t.Fatalf("got %q", got)
	}
	if got := normalizePath(repo, "a/b.go"); got != "a/b.go" {
		t.Fatalf("got %q", got)
	}
}

// Ambient capture alone (commands, prompts, files outside the repository)
// is no evidence of work here: no file is written, so none is published,
// and the events stay in the local archive.
func TestRunWritesNoRecordOfAmbientCapture(t *testing.T) {
	root := initRepo(t)
	for _, ev := range []store.Event{
		{Kind: store.KindCommand, At: t0, Cmd: "gh release view --repo provasign/shale"},
		{Kind: store.KindPrompt, At: t0.Add(time.Second), Text: "check the trial"},
		{Kind: store.KindFileTouch, At: t0.Add(2 * time.Second), Path: "/private/tmp/elsewhere/app.py", Op: "edit"},
	} {
		if err := store.AppendEvent(root, "SESSAMBIENT01", ev); err != nil {
			t.Fatal(err)
		}
	}
	res, err := Run(Options{RepoRoot: root, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Written) != 0 {
		t.Fatalf("written = %v, want nothing", res.Written)
	}
	if _, err := os.Stat(filepath.Join(root, ".shale", "local", "archive", "SESSAMBIENT01.jsonl")); err != nil {
		t.Fatalf("events not kept in the archive: %v", err)
	}

	// After a finalized session, a remnant touching only files elsewhere
	// mints no continuation either.
	seedFullSession(t, root, "SESSDONE01")
	if _, err := Run(Options{RepoRoot: root, Now: now}); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendEvent(root, "SESSDONE01", store.Event{Kind: store.KindFileTouch, At: now.Add(time.Minute), Path: "/private/tmp/elsewhere/b.py", Op: "edit"}); err != nil {
		t.Fatal(err)
	}
	if res, err = Run(Options{RepoRoot: root, Now: now.Add(2 * time.Minute)}); err != nil || len(res.Written) != 0 {
		t.Fatalf("remnant: written %v, %v", res.Written, err)
	}
}

// Sessions limits a run to the named sessions; the others stay open.
func TestRunFoldsOnlyTheNamedSessions(t *testing.T) {
	root := initRepo(t)
	for _, id := range []string{"SESSMINE01", "SESSOTHER01"} {
		if err := store.AppendEvent(root, id, store.Event{Kind: store.KindIntent, At: t0, Title: "work of " + id}); err != nil {
			t.Fatal(err)
		}
	}
	res, err := Run(Options{RepoRoot: root, Now: now, Sessions: []string{"SESSMINE01"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Written) != 1 || !strings.Contains(res.Written[0], "SESSMINE01") {
		t.Fatalf("written = %v, want SESSMINE01 only", res.Written)
	}
	if _, err := os.Stat(store.SessionPath(root, "SESSOTHER01")); err != nil {
		t.Fatalf("the other session's log was taken: %v", err)
	}
}

// At commit time only closed tasks are recorded: the task in progress keeps
// its events in the log, with the session's metadata, and is recorded under
// the next name once it is closed.
func TestRunClosedOnlyKeepsTheOpenTask(t *testing.T) {
	root := initRepo(t)
	for _, ev := range []store.Event{
		{Kind: store.KindSessionMeta, At: t0, Tool: "codex"},
		{Kind: store.KindIntent, At: t0.Add(time.Minute), Title: "Create tasks"},
		{Kind: store.KindCompletion, At: t0.Add(2 * time.Minute), Note: "created"},
		{Kind: store.KindIntent, At: t0.Add(3 * time.Minute), Title: "List tasks"},
	} {
		if err := store.AppendEvent(root, "SESSOPEN01", ev); err != nil {
			t.Fatal(err)
		}
	}
	res, err := Run(Options{RepoRoot: root, Now: now, ClosedOnly: true})
	if err != nil || len(res.Written) != 1 || !strings.HasSuffix(res.Written[0], "SESSOPEN01.yaml") {
		t.Fatalf("written %v, %v; want the closed task only", res.Written, err)
	}
	open, err := store.ReadEvents(store.SessionPath(root, "SESSOPEN01"))
	if err != nil || len(open) != 2 || open[0].Kind != store.KindSessionMeta || open[1].Title != "List tasks" {
		t.Fatalf("open log = %+v, %v; want the metadata and the open task", open, err)
	}

	// Nothing closed yet: nothing is recorded, the log stays as it is.
	if res, err = Run(Options{RepoRoot: root, Now: now, ClosedOnly: true}); err != nil || len(res.Written) != 0 {
		t.Fatalf("second run: %v, %v", res.Written, err)
	}
	// The session ends for good without a done: the task is closed.
	if err := store.AppendEvent(root, "SESSOPEN01", store.Event{Kind: store.KindSessionClose, At: t0.Add(4 * time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if res, err = Run(Options{RepoRoot: root, Now: now, ClosedOnly: true}); err != nil || len(res.Written) != 1 || !strings.HasSuffix(res.Written[0], "SESSOPEN01-c2.yaml") {
		t.Fatalf("after the session ended: %v, %v", res.Written, err)
	}
	s, _ := store.Load(filepath.Join(root, res.Written[0]))
	if s.Intent == nil || s.Intent.Title != "List tasks" || s.Completion == nil || !strings.Contains(s.Completion.Note, "ended without shale done") || s.Agent.Tool != "codex" {
		t.Fatalf("record = %+v / %+v / %q", s.Intent, s.Completion, s.Agent.Tool)
	}
}

// A conversation that exits and is resumed (claude --resume, or each turn
// of claude -p --resume) closes and reopens the same session: the close
// ended nothing. The request declared before it is the task the resumed
// work completes, and one declared again after it restates that request.
func TestRunResumedSessionKeepsItsRequest(t *testing.T) {
	root := initRepo(t)
	for _, ev := range []store.Event{
		{Kind: store.KindSessionMeta, At: t0, Tool: "claude-code"},
		{Kind: store.KindIntent, At: t0.Add(time.Minute), Title: "Limit loans to 3"},
		{Kind: store.KindSessionEnd, At: t0.Add(2 * time.Minute)},
		{Kind: store.KindSessionClose, At: t0.Add(2 * time.Minute)},
		{Kind: store.KindSessionMeta, At: t0.Add(3 * time.Minute), Tool: "claude-code"},
		{Kind: store.KindIntent, At: t0.Add(4 * time.Minute), Title: "Limit loans to 3 at a time"},
		{Kind: store.KindSessionClose, At: t0.Add(5 * time.Minute)},
		{Kind: store.KindSessionMeta, At: t0.Add(6 * time.Minute), Tool: "claude-code"},
		{Kind: store.KindFileTouch, At: t0.Add(7 * time.Minute), Path: filepath.Join(root, "lend.go"), Op: "edit"},
		{Kind: store.KindCompletion, At: t0.Add(8 * time.Minute), Note: "limit built"},
	} {
		if err := store.AppendEvent(root, "SESSRESUME1", ev); err != nil {
			t.Fatal(err)
		}
	}
	res, err := Run(Options{RepoRoot: root, Now: now, ClosedOnly: true})
	if err != nil || len(res.Written) != 1 || !strings.HasSuffix(res.Written[0], "SESSRESUME1.yaml") {
		t.Fatalf("written %v, %v; want one record", res.Written, err)
	}
	s, _ := store.Load(filepath.Join(root, res.Written[0]))
	if s.Intent == nil || s.Intent.Title != "Limit loans to 3 at a time" || s.Completion == nil || s.Completion.Note != "limit built" || len(s.Files) != 1 {
		t.Fatalf("record = %+v / %+v / %v", s.Intent, s.Completion, s.Files)
	}
}

// The last close, with nothing after it, still ends the session.
func TestResumed(t *testing.T) {
	evs := []store.Event{{Kind: store.KindIntent}, {Kind: store.KindSessionClose}, {Kind: store.KindSessionMeta}, {Kind: store.KindSessionEnd}, {Kind: store.KindSessionClose}}
	if !store.Resumed(evs, 1) || store.Resumed(evs, 4) {
		t.Fatalf("Resumed = %v, %v; want true, false", store.Resumed(evs, 1), store.Resumed(evs, 4))
	}
}

// A project's own private terms never reach committed evidence: not in the
// request, the notes, the commands, the file paths or the branch, from any of
// the three term lists.
func TestRunMasksPrivateTerms(t *testing.T) {
	root := initRepo(t)
	cmd := exec.Command("git", "checkout", "-q", "-b", "acme-rollout")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if err := os.MkdirAll(filepath.Join(root, ".shale", "local"), 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(root, ".shale", "redact.txt"), []byte("# team terms\nAcme\n"), 0o644)
	os.WriteFile(filepath.Join(root, ".shale", "local", "redact.txt"), []byte("Bluebird\n"), 0o644)
	private := filepath.Join(t.TempDir(), "private.txt")
	os.WriteFile(private, []byte("orion-x\n"), 0o644)
	t.Setenv("SHALE_REDACT_FILE", private)

	for _, ev := range []store.Event{
		{Kind: store.KindIntent, At: t0, Title: "Compare ACME and bluebird specs", Body: "Read Orion-X too."},
		{Kind: store.KindFileTouch, At: t0.Add(time.Second), Path: "notes/bluebird.md", Op: "write"},
		{Kind: store.KindCommand, At: t0.Add(time.Minute), Cmd: "go test ./... -run acme", ExitCode: intPtr(0)},
		{Kind: store.KindNote, At: t0.Add(90 * time.Second), Text: "orion-x read"},
		{Kind: store.KindCompletion, At: t0.Add(2 * time.Minute), Note: "Acme done; acmes and bluebirds untouched"},
	} {
		if err := store.AppendEvent(root, "SESSTERM1", ev); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := Run(Options{RepoRoot: root, Now: now}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(root, ".shale", "SESSTERM1.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	low := strings.ToLower(string(raw))
	for _, banned := range []string{"acme ", "acme-", "acme;", "bluebird.", "bluebird ", "orion-x"} {
		if strings.Contains(low, banned) {
			t.Fatalf("evidence leaked %q:\n%s", banned, raw)
		}
	}
	// Whole words only: "acmes" and "bluebirds" are other words.
	for _, kept := range []string{"acmes", "bluebirds", "[redacted:term]"} {
		if !strings.Contains(low, kept) {
			t.Fatalf("evidence lost %q:\n%s", kept, raw)
		}
	}
}

// A term list that is named but missing stops the record from being written.
func TestRunRefusesWithoutNamedTerms(t *testing.T) {
	root := initRepo(t)
	t.Setenv("SHALE_REDACT_FILE", filepath.Join(t.TempDir(), "gone.txt"))
	for _, ev := range []store.Event{
		{Kind: store.KindIntent, At: t0, Title: "Anything"},
		{Kind: store.KindCompletion, At: t0.Add(time.Minute), Note: "done"},
	} {
		if err := store.AppendEvent(root, "SESSTERM2", ev); err != nil {
			t.Fatal(err)
		}
	}
	// The session is skipped, not lost: it folds once the list is back.
	if _, err := Run(Options{RepoRoot: root, Now: now}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ".shale", "SESSTERM2.yaml")); err == nil {
		t.Fatal("a record was written without the named term list")
	}
	list := os.Getenv("SHALE_REDACT_FILE")
	if err := os.WriteFile(list, []byte("anything\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(Options{RepoRoot: root, Now: now}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ".shale", "SESSTERM2.yaml")); err != nil {
		t.Fatal("the session was lost once the list was back")
	}
}
