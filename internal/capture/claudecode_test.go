package capture

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/provasign/shale/internal/store"
)

var now = time.Date(2026, 6, 9, 14, 5, 2, 0, time.UTC)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "claude-code", name))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestParseSessionStart(t *testing.T) {
	events, sid, cwd := ParseClaudeCode(fixture(t, "session_start.json"), now)
	if sid != "a1b2c3d4-5678-90ab-cdef-112233445566" {
		t.Fatalf("session id = %q", sid)
	}
	if cwd != "/work/my-repo" {
		t.Fatalf("cwd = %q", cwd)
	}
	if len(events) != 1 || events[0].Kind != store.KindSessionMeta || events[0].Tool != "claude-code" {
		t.Fatalf("events = %+v", events)
	}
}

func TestParseUserPrompt(t *testing.T) {
	events, _, _ := ParseClaudeCode(fixture(t, "user_prompt.json"), now)
	if len(events) != 1 || events[0].Kind != store.KindPrompt {
		t.Fatalf("events = %+v", events)
	}
	// Spec rule 5: prompts are transcript material, never intent.
	if events[0].Kind == store.KindIntent {
		t.Fatal("prompt must never become an intent event")
	}
	if events[0].Text == "" {
		t.Fatal("prompt text lost")
	}
}

func TestParseFileTouches(t *testing.T) {
	cases := []struct {
		fixture string
		path    string
		op      string
	}{
		{"post_tool_write.json", "internal/auth/ratelimit.go", "write"},
		{"post_tool_edit.json", "internal/auth/login.go", "edit"},
	}
	for _, tc := range cases {
		events, _, _ := ParseClaudeCode(fixture(t, tc.fixture), now)
		if len(events) != 1 || events[0].Kind != store.KindFileTouch {
			t.Fatalf("%s: events = %+v", tc.fixture, events)
		}
		if events[0].Path != tc.path || events[0].Op != tc.op {
			t.Fatalf("%s: got %s/%s", tc.fixture, events[0].Path, events[0].Op)
		}
	}
}

func TestParseBashCommand(t *testing.T) {
	events, _, _ := ParseClaudeCode(fixture(t, "post_tool_bash.json"), now)
	if len(events) != 1 || events[0].Kind != store.KindCommand {
		t.Fatalf("events = %+v", events)
	}
	if events[0].Cmd != "go test ./internal/auth/..." {
		t.Fatalf("cmd = %q", events[0].Cmd)
	}
	if events[0].ExitCode == nil || *events[0].ExitCode != 0 {
		t.Fatalf("exit code = %v", events[0].ExitCode)
	}
}

func TestParseBashWithoutExitCode(t *testing.T) {
	raw := []byte(`{"session_id":"s","cwd":"/w","hook_event_name":"PostToolUse","tool_name":"Bash","tool_input":{"command":"ls"},"tool_response":{"stdout":"ok"}}`)
	events, _, _ := ParseClaudeCode(raw, now)
	if len(events) != 1 || events[0].ExitCode != nil {
		t.Fatalf("exit code must be omitted when unknown (honest provenance): %+v", events)
	}
}

func TestParseStop(t *testing.T) {
	events, _, _ := ParseClaudeCode(fixture(t, "stop.json"), now)
	if len(events) != 1 || events[0].Kind != store.KindSessionEnd {
		t.Fatalf("events = %+v", events)
	}
}

func TestParseTotalOnGarbage(t *testing.T) {
	for _, raw := range []string{"", "not json", `{"hook_event_name":"SomethingNew"}`, `{"hook_event_name":"PostToolUse","tool_name":"Bash"}`} {
		events, _, _ := ParseClaudeCode([]byte(raw), now)
		if len(events) != 0 {
			t.Fatalf("garbage %q produced events %+v", raw, events)
		}
	}
}
