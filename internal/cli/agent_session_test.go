package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/provasign/shale/internal/store"
)

// TestAgentsShareARepository: two agents work in one repository at once.
// Each CLI call lands in its own agent's session, however the hooks moved
// .shale/local/current, and a session the hooks never saw says its tool.
func TestAgentsShareARepository(t *testing.T) {
	root := chrepo(t)
	// Claude Code's hooks ran last: the pointer names its session.
	if err := store.SetCurrentSession(root, "claude-1"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODEX_THREAD_ID", "codex-1")
	if code, _, errOut := run(t, "", "intent", "Write the review"); code != 0 {
		t.Fatalf("codex intent: %s", errOut)
	}
	t.Setenv("CODEX_THREAD_ID", "")
	t.Setenv("CLAUDE_CODE_SESSION_ID", "claude-1")
	if code, _, errOut := run(t, "", "intent", "Fix the importer"); code != 0 {
		t.Fatalf("claude intent: %s", errOut)
	}

	codex, err := store.ReadEvents(store.SessionPath(root, "codex-1"))
	if err != nil || len(codex) != 2 || codex[0].Kind != store.KindSessionMeta || codex[0].Tool != "codex" || codex[1].Title != "Write the review" {
		t.Fatalf("codex session = %+v (%v); want its tool, then its own request", codex, err)
	}
	claude, err := store.ReadEvents(store.SessionPath(root, "claude-1"))
	if err != nil {
		t.Fatal(err)
	}
	var titles []string
	for _, ev := range claude {
		if ev.Kind == store.KindIntent {
			titles = append(titles, ev.Title)
		}
	}
	if strings.Join(titles, "|") != "Fix the importer" {
		t.Fatalf("claude session requests = %v; want only its own", titles)
	}
	if got := store.CurrentSession(root); got != "claude-1" {
		t.Fatalf("pointer = %q; a call with its own id leaves it alone", got)
	}
}

// An id that could leave .shale/local is not a session id.
func TestAgentSessionRejectsPaths(t *testing.T) {
	root := chrepo(t)
	t.Setenv("CLAUDE_CODE_SESSION_ID", "../../escape")
	id, err := agentSession(root)
	if err != nil || strings.Contains(id, "/") || strings.Contains(id, "..") {
		t.Fatalf("id = %q, %v", id, err)
	}
}

// shale done finalizes the calling agent's session only: another agent's
// open session in the same checkout is not committed with it.
func TestDoneFinalizesOnlyItsOwnSession(t *testing.T) {
	root := chrepo(t)
	if err := os.MkdirAll(filepath.Join(root, ".shale"), 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(root, ".shale", "schema-version"), []byte("0\n"), 0o644)
	t.Setenv("CLAUDE_CODE_SESSION_ID", "claude-1")
	if code, _, errOut := run(t, "", "intent", "Fix the importer"); code != 0 {
		t.Fatalf("claude intent: %s", errOut)
	}
	t.Setenv("CLAUDE_CODE_SESSION_ID", "")
	t.Setenv("CODEX_THREAD_ID", "codex-1")
	if code, _, errOut := run(t, "", "intent", "Run the trial"); code != 0 {
		t.Fatalf("codex intent: %s", errOut)
	}
	if code, _, errOut := run(t, "", "done", "--note", "trial step"); code != 0 {
		t.Fatalf("codex done: %s", errOut)
	}
	if _, err := os.Stat(filepath.Join(root, ".shale", "codex-1.yaml")); err != nil {
		t.Fatalf("the calling agent's session was not finalized: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".shale", "claude-1.yaml")); err == nil {
		t.Fatal("done finalized another agent's session")
	}
	if _, err := os.Stat(store.SessionPath(root, "claude-1")); err != nil {
		t.Fatalf("the other agent's log was taken: %v", err)
	}
}
