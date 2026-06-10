package capture

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/provasign/shale/internal/store"
)

func copilotFixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "copilot", name))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestParseCopilotSessionStart(t *testing.T) {
	events, sid, cwd := ParseCopilot(copilotFixture(t, "session_start.json"), now)
	if sid != "copilot-session-001" {
		t.Fatalf("session id = %q", sid)
	}
	if cwd != "/work/my-repo" {
		t.Fatalf("cwd = %q", cwd)
	}
	if len(events) != 1 || events[0].Kind != store.KindSessionMeta || events[0].Tool != "copilot" {
		t.Fatalf("events = %+v", events)
	}
}

func TestParseCopilotPrompt(t *testing.T) {
	events, _, _ := ParseCopilot(copilotFixture(t, "user_prompt.json"), now)
	if len(events) != 1 || events[0].Kind != store.KindPrompt || events[0].Text == "" {
		t.Fatalf("events = %+v", events)
	}
}

func TestParseCopilotFileTouch(t *testing.T) {
	events, _, _ := ParseCopilot(copilotFixture(t, "post_tool_write.json"), now)
	if len(events) != 1 || events[0].Kind != store.KindFileTouch || events[0].Path != "internal/auth/auth_test.go" || events[0].Op != "write" {
		t.Fatalf("events = %+v", events)
	}
}

func TestParseCopilotBashCommand(t *testing.T) {
	events, _, _ := ParseCopilot(copilotFixture(t, "post_tool_bash.json"), now)
	if len(events) != 1 || events[0].Kind != store.KindCommand {
		t.Fatalf("events = %+v", events)
	}
	if events[0].Cmd != "go test ./..." {
		t.Fatalf("cmd = %q", events[0].Cmd)
	}
	if events[0].ExitCode == nil || *events[0].ExitCode != 0 {
		t.Fatalf("exit code = %v", events[0].ExitCode)
	}
}

func TestParseCopilotSessionEnd(t *testing.T) {
	events, _, _ := ParseCopilot(copilotFixture(t, "session_end.json"), now)
	if len(events) != 1 || events[0].Kind != store.KindSessionEnd {
		t.Fatalf("events = %+v", events)
	}
}

func TestParseCopilotSnakeCasePayload(t *testing.T) {
	raw := []byte(`{"thread_id":"s","workspace_root":"/w","hook_event_name":"postToolUse","tool_name":"Bash","tool_input":{"command":"go test ./internal/auth"},"tool_response":{"exit_code":1}}`)
	events, sid, cwd := ParseCopilot(raw, now)
	if sid != "s" || cwd != "/w" || len(events) != 1 || events[0].Cmd != "go test ./internal/auth" {
		t.Fatalf("sid=%q cwd=%q events=%+v", sid, cwd, events)
	}
	if events[0].ExitCode == nil || *events[0].ExitCode != 1 {
		t.Fatalf("exit code = %v", events[0].ExitCode)
	}
}

func TestParseCopilotTotalOnGarbage(t *testing.T) {
	for _, raw := range []string{"", "not json", `{"hookEventName":"somethingNew"}`, `{"hookEventName":"postToolUse","toolName":"Bash"}`} {
		events, _, _ := ParseCopilot([]byte(raw), now)
		if len(events) != 0 {
			t.Fatalf("garbage %q produced events %+v", raw, events)
		}
	}
}
