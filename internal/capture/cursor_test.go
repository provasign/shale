package capture

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/provasign/shale/internal/store"
)

func cursorFixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "cursor", name))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestParseCursorSessionStart(t *testing.T) {
	events, sid, cwd := ParseCursor(cursorFixture(t, "session_start.json"), now)
	if sid != "cursor-session-001" {
		t.Fatalf("session id = %q", sid)
	}
	if cwd != "/work/my-repo" {
		t.Fatalf("cwd = %q", cwd)
	}
	if len(events) != 1 || events[0].Kind != store.KindSessionMeta || events[0].Tool != "cursor" {
		t.Fatalf("events = %+v", events)
	}
}

func TestParseCursorPrompt(t *testing.T) {
	events, _, _ := ParseCursor(cursorFixture(t, "user_prompt.json"), now)
	if len(events) != 1 || events[0].Kind != store.KindPrompt || events[0].Text == "" {
		t.Fatalf("events = %+v", events)
	}
}

func TestParseCursorAfterFileEdit(t *testing.T) {
	events, _, _ := ParseCursor(cursorFixture(t, "after_file_edit.json"), now)
	if len(events) != 1 || events[0].Kind != store.KindFileTouch || events[0].Path != "internal/auth/auth.go" {
		t.Fatalf("events = %+v", events)
	}
}

func TestParseCursorBashCommand(t *testing.T) {
	events, _, _ := ParseCursor(cursorFixture(t, "post_tool_bash.json"), now)
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

func TestParseCursorSessionEnd(t *testing.T) {
	events, _, _ := ParseCursor(cursorFixture(t, "session_end.json"), now)
	if len(events) != 1 || events[0].Kind != store.KindSessionEnd {
		t.Fatalf("events = %+v", events)
	}
}

func TestParseCursorSnakeCasePayload(t *testing.T) {
	raw := []byte(`{"session_id":"s","cwd":"/w","hook_event_name":"afterFileEdit","tool_info":{"file_path":"main.go"}}`)
	events, sid, cwd := ParseCursor(raw, now)
	if sid != "s" || cwd != "/w" || len(events) != 1 || events[0].Path != "main.go" {
		t.Fatalf("sid=%q cwd=%q events=%+v", sid, cwd, events)
	}
}

func TestParseCursorTotalOnGarbage(t *testing.T) {
	for _, raw := range []string{"", "not json", `{"hookEventName":"somethingNew"}`, `{"hookEventName":"postToolUse","toolName":"Bash"}`} {
		events, _, _ := ParseCursor([]byte(raw), now)
		if len(events) != 0 {
			t.Fatalf("garbage %q produced events %+v", raw, events)
		}
	}
}
