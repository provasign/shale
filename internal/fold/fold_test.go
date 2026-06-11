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
