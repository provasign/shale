package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/provasign/shale/internal/store"
)

// chrepo creates a temp git repo, chdirs into it, and isolates HOME.
func chrepo(t *testing.T) string {
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
	t.Chdir(root)
	t.Setenv("HOME", t.TempDir())
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func run(t *testing.T, stdin string, args ...string) (int, string, string) {
	t.Helper()
	var out, errBuf bytes.Buffer
	code := Run(args, strings.NewReader(stdin), &out, &errBuf)
	return code, out.String(), errBuf.String()
}

func TestUsageAndUnknown(t *testing.T) {
	code, _, errOut := run(t, "")
	if code != 1 || !strings.Contains(errOut, "Usage:") {
		t.Fatalf("bare run: code=%d err=%q", code, errOut)
	}
	code, _, errOut = run(t, "", "frobnicate")
	if code != 1 || !strings.Contains(errOut, "unknown command") {
		t.Fatalf("unknown: code=%d", code)
	}
	code, out, _ := run(t, "", "version")
	if code != 0 || !strings.Contains(out, "shale") {
		t.Fatalf("version: code=%d out=%q", code, out)
	}
	code, out, _ = run(t, "", "help")
	if code != 0 || !strings.Contains(out, "shale intent") {
		t.Fatalf("help: code=%d", code)
	}
}

func TestInitThenDoctor(t *testing.T) {
	chrepo(t)
	code, out, errOut := run(t, "", "init", "--privacy", "redacted")
	if code != 0 {
		t.Fatalf("init failed: %s / %s", out, errOut)
	}
	for _, must := range []string{"Steering prompt", "capture hooks", ".shale/", "pre-push hook"} {
		if !strings.Contains(out, must) {
			t.Errorf("init output missing %q:\n%s", must, out)
		}
	}

	// Idempotent re-run.
	code, out2, _ := run(t, "", "init")
	if code != 0 || !strings.Contains(out2, "already present") {
		t.Fatalf("re-init: code=%d out=%s", code, out2)
	}

	code, out3, _ := run(t, "", "doctor")
	// Only "capture events seen" should fail on a fresh init.
	if code != 1 || !strings.Contains(out3, "✗ capture events seen") {
		t.Fatalf("doctor: code=%d out=%s", code, out3)
	}
	if strings.Count(out3, "✗") != 1 {
		t.Fatalf("doctor: expected exactly 1 failing check after init:\n%s", out3)
	}

	code, _, errOut = run(t, "", "init", "--privacy", "shouty")
	if code != 1 || !strings.Contains(errOut, "invalid --privacy") {
		t.Fatalf("bad privacy accepted: code=%d", code)
	}
}

func TestIntentDoneNoteFlow(t *testing.T) {
	root := chrepo(t)
	code, out, errOut := run(t, "", "intent", "Add rate limiting", "--body", "Redis, 10/min")
	if code != 0 || !strings.Contains(out, "✓ intent recorded") {
		t.Fatalf("intent: code=%d out=%q err=%q", code, out, errOut)
	}
	if strings.Count(out, "\n") != 1 {
		t.Fatalf("intent ack must be a single line (context budget): %q", out)
	}

	code, out, _ = run(t, "", "done", "--note", "limiter done", "--tokens-in", "1000", "--tokens-out", "500", "--model", "claude-fable-5", "--iterations", "2")
	if code != 0 || !strings.Contains(out, "✓ completion recorded") {
		t.Fatalf("done: code=%d out=%q", code, out)
	}

	code, _, _ = run(t, "", "note", "manual annotation")
	if code != 0 {
		t.Fatal("note failed")
	}

	sid := store.CurrentSession(root)
	if sid == "" {
		t.Fatal("no current session")
	}
	events, err := store.ReadEvents(store.SessionPath(root, sid))
	if err != nil || len(events) != 3 {
		t.Fatalf("events = %d (%v)", len(events), err)
	}
	if events[0].Kind != store.KindIntent || events[1].Kind != store.KindCompletion || events[2].Kind != store.KindNote {
		t.Fatalf("kinds = %s %s %s", events[0].Kind, events[1].Kind, events[2].Kind)
	}
	// Flags after the positional title must be parsed, not folded into it —
	// the documented call shape is `shale intent "<title>" --body "..."`.
	if events[0].Title != "Add rate limiting" || events[0].Body != "Redis, 10/min" {
		t.Fatalf("intent parsed wrong: title=%q body=%q", events[0].Title, events[0].Body)
	}
	if events[1].Model != "claude-fable-5" || events[1].TokensIn != 1000 {
		t.Fatalf("done parsed wrong: model=%q tokensIn=%d", events[1].Model, events[1].TokensIn)
	}

	// Missing title is a usage error.
	code, _, _ = run(t, "", "intent")
	if code != 1 {
		t.Fatal("empty intent accepted")
	}
}

func TestCaptureFailOpenAlways(t *testing.T) {
	chrepo(t)
	cases := []struct {
		name  string
		stdin string
		args  []string
	}{
		{"garbage stdin", "not json at all", []string{"capture", "claude-code"}},
		{"unknown adapter", "{}", []string{"capture", "psychic-agent"}},
		{"no args", "", []string{"capture"}},
		{"empty payload", "", []string{"capture", "claude-code"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, _, _ := run(t, tc.stdin, tc.args...)
			if code != 0 {
				t.Fatalf("capture must exit 0 on every error, got %d", code)
			}
		})
	}
}

func TestCaptureWritesEvents(t *testing.T) {
	root := chrepo(t)
	run(t, "", "init")
	payload := `{"session_id":"sess-xyz","cwd":` + strconv.Quote(root) + `,"hook_event_name":"PostToolUse","tool_name":"Write","tool_input":{"file_path":"a.go"}}`
	code, _, _ := run(t, payload, "capture", "claude-code")
	if code != 0 {
		t.Fatal("capture failed")
	}
	events, err := store.ReadEvents(store.SessionPath(root, "sess-xyz"))
	if err != nil || len(events) != 1 || events[0].Path != "a.go" {
		t.Fatalf("events = %+v (%v)", events, err)
	}
	if store.CurrentSession(root) != "sess-xyz" {
		t.Fatal("current session not tracked")
	}
}

func TestCaptureWritesCodexEvents(t *testing.T) {
	root := chrepo(t)
	run(t, "", "init")
	payload := `{"session_id":"codex-sess","cwd":` + strconv.Quote(root) + `,"hook_event_name":"PostToolUse","tool_name":"Bash","tool_input":{"command":"go test ./..."},"tool_response":{"exit_code":0}}`
	code, _, _ := run(t, payload, "capture", "codex")
	if code != 0 {
		t.Fatal("capture failed")
	}
	events, err := store.ReadEvents(store.SessionPath(root, "codex-sess"))
	if err != nil || len(events) != 1 || events[0].Cmd != "go test ./..." {
		t.Fatalf("events = %+v (%v)", events, err)
	}
	if store.CurrentSession(root) != "codex-sess" {
		t.Fatal("current session not tracked")
	}
}

func TestCaptureWritesCursorEvents(t *testing.T) {
	root := chrepo(t)
	run(t, "", "init")
	payload := `{"sessionId":"cursor-sess","cwd":` + strconv.Quote(root) + `,"hookEventName":"afterFileEdit","toolInfo":{"filePath":"main.go"}}`
	code, _, _ := run(t, payload, "capture", "cursor")
	if code != 0 {
		t.Fatal("capture failed")
	}
	events, err := store.ReadEvents(store.SessionPath(root, "cursor-sess"))
	if err != nil || len(events) != 1 || events[0].Path != "main.go" {
		t.Fatalf("events = %+v (%v)", events, err)
	}
	if store.CurrentSession(root) != "cursor-sess" {
		t.Fatal("current session not tracked")
	}
}

func TestCaptureWritesCopilotEvents(t *testing.T) {
	root := chrepo(t)
	run(t, "", "init")
	payload := `{"sessionId":"copilot-sess","cwd":` + strconv.Quote(root) + `,"hookEventName":"postToolUse","toolName":"Write","toolInput":{"filePath":"main.go"}}`
	code, _, _ := run(t, payload, "capture", "copilot")
	if code != 0 {
		t.Fatal("capture failed")
	}
	events, err := store.ReadEvents(store.SessionPath(root, "copilot-sess"))
	if err != nil || len(events) != 1 || events[0].Path != "main.go" {
		t.Fatalf("events = %+v (%v)", events, err)
	}
	if store.CurrentSession(root) != "copilot-sess" {
		t.Fatal("current session not tracked")
	}
}

func TestCaptureIgnoresNonShaleRepos(t *testing.T) {
	root := chrepo(t) // no `shale init` → no .shale/ scaffold
	payload := `{"session_id":"s","cwd":` + strconv.Quote(root) + `,"hook_event_name":"PostToolUse","tool_name":"Write","tool_input":{"file_path":"a.go"}}`
	code, _, _ := run(t, payload, "capture", "claude-code")
	if code != 0 {
		t.Fatal("capture failed")
	}
	if _, err := os.Stat(filepath.Join(root, ".shale")); !os.IsNotExist(err) {
		t.Fatal("capture must not create .shale/ in repos that did not opt in")
	}
}

func TestFinalizeAndRenderLocalEndToEnd(t *testing.T) {
	root := chrepo(t)
	run(t, "", "init")

	// Simulate a session: hooks + intent + done.
	payloads := []string{
		`{"session_id":"e2e-sess","cwd":` + strconv.Quote(root) + `,"hook_event_name":"SessionStart","model":"claude-fable-5"}`,
		`{"session_id":"e2e-sess","cwd":` + strconv.Quote(root) + `,"hook_event_name":"UserPromptSubmit","prompt":"add rate limiting"}`,
		`{"session_id":"e2e-sess","cwd":` + strconv.Quote(root) + `,"hook_event_name":"PostToolUse","tool_name":"Write","tool_input":{"file_path":"ratelimit.go"}}`,
		`{"session_id":"e2e-sess","cwd":` + strconv.Quote(root) + `,"hook_event_name":"PostToolUse","tool_name":"Bash","tool_input":{"command":"go test ./..."},"tool_response":{"exit_code":0}}`,
	}
	for _, p := range payloads {
		if code, _, _ := run(t, p, "capture", "claude-code"); code != 0 {
			t.Fatal("capture failed")
		}
	}
	run(t, "", "intent", "Add rate limiting", "--body", "Redis based")
	run(t, "", "done", "--note", "done", "--tokens-in", "1000", "--tokens-out", "200", "--model", "claude-fable-5")

	code, out, errOut := run(t, "", "finalize")
	if code != 0 || !strings.Contains(out, "finalized") {
		t.Fatalf("finalize: code=%d out=%q err=%q", code, out, errOut)
	}
	if _, err := os.Stat(filepath.Join(root, ".shale", "e2e-sess.yaml")); err != nil {
		t.Fatal("shale YAML not written")
	}

	// Idempotent.
	code, out, _ = run(t, "", "finalize")
	if code != 0 || !strings.Contains(out, "nothing to finalize") {
		t.Fatalf("re-finalize: %q", out)
	}

	code, out, errOut = run(t, "", "render", "--local")
	if code != 0 {
		t.Fatalf("render --local: %s", errOut)
	}
	for _, must := range []string{"Add rate limiting", "ratelimit.go", "✅"} {
		if !strings.Contains(out, must) {
			t.Errorf("card missing %q:\n%s", must, out)
		}
	}
}

func TestFinalizeAutoCommit(t *testing.T) {
	root := chrepo(t)
	run(t, "", "init")
	// Commit the scaffold so the evidence commit is isolated.
	for _, args := range [][]string{{"add", "-A"}, {"commit", "-m", "scaffold", "--no-verify"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		cmd.Run()
	}
	run(t, "", "intent", "Quick fix")
	run(t, "", "done", "--note", "fixed")

	code, _, errOut := run(t, "", "finalize", "--auto-commit")
	if code != 0 {
		t.Fatalf("finalize --auto-commit: %s", errOut)
	}
	log := exec.Command("git", "log", "-1", "--pretty=%s")
	log.Dir = root
	out, _ := log.Output()
	if !strings.Contains(string(out), "chore(shale): session evidence") {
		t.Fatalf("evidence commit missing, last commit: %s", out)
	}
}

func TestRenderPRRequiresConfig(t *testing.T) {
	chrepo(t)
	t.Setenv("GITHUB_EVENT_PATH", "")
	code, _, errOut := run(t, "", "render")
	if code != 1 || !strings.Contains(errOut, "--pr") {
		t.Fatalf("render without pr: code=%d err=%q", code, errOut)
	}
	t.Setenv("SHALE_REPO", "")
	t.Setenv("GITHUB_REPOSITORY", "")
	t.Setenv("SHALE_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")
	code, _, errOut = run(t, "", "render", "--pr", "7")
	if code != 1 || !strings.Contains(errOut, "SHALE_REPO") {
		t.Fatalf("render without env: code=%d err=%q", code, errOut)
	}
}

func TestDetectPRNumber(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "event.json")
	os.WriteFile(path, []byte(`{"pull_request":{"number":42}}`), 0o644)
	t.Setenv("GITHUB_EVENT_PATH", path)
	if n := detectPRNumber(); n != 42 {
		t.Fatalf("detectPRNumber = %d", n)
	}
	t.Setenv("GITHUB_EVENT_PATH", "")
	if n := detectPRNumber(); n != 0 {
		t.Fatalf("no event path should yield 0, got %d", n)
	}
}
