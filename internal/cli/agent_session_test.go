package cli

import (
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
