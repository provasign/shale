package capture

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/provasign/shale/internal/store"
)

func codexFixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "codex", name))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestParseCodexSessionStart(t *testing.T) {
	events, sid, cwd := ParseCodex(codexFixture(t, "session_start.json"), now)
	if sid != "019eb3b5-731e-7d50-b90e-07bfd63ba927" {
		t.Fatalf("session id = %q", sid)
	}
	if cwd != "/work/my-repo" {
		t.Fatalf("cwd = %q", cwd)
	}
	if len(events) != 1 || events[0].Kind != store.KindSessionMeta || events[0].Tool != "codex" {
		t.Fatalf("events = %+v", events)
	}
}

func TestParseCodexUserPrompt(t *testing.T) {
	events, _, _ := ParseCodex(codexFixture(t, "user_prompt.json"), now)
	if len(events) != 1 || events[0].Kind != store.KindPrompt || events[0].Text == "" {
		t.Fatalf("events = %+v", events)
	}
}

func TestParseCodexBashCommand(t *testing.T) {
	events, _, _ := ParseCodex(codexFixture(t, "post_tool_bash.json"), now)
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

func TestParseCodexApplyPatchTouches(t *testing.T) {
	events, _, _ := ParseCodex(codexFixture(t, "post_tool_apply_patch.json"), now)
	if len(events) != 3 {
		t.Fatalf("events = %+v", events)
	}
	want := []struct {
		path string
		op   string
	}{
		{"main.go", "edit"},
		{"internal/auth/policy.go", "write"},
		{"old.go", "delete"},
	}
	for i := range want {
		if events[i].Kind != store.KindFileTouch || events[i].Path != want[i].path || events[i].Op != want[i].op {
			t.Fatalf("event[%d] = %+v", i, events[i])
		}
	}
}

func TestParseCodexApplyPatchPayloadVariants(t *testing.T) {
	cases := []string{
		`{"patch":"*** Begin Patch\n*** Update File: README.md\n@@\n+note\n*** End Patch\n"}`,
		`{"input":"*** Begin Patch\n*** Update File: README.md\n@@\n+note\n*** End Patch\n"}`,
		`{"cmd":"*** Begin Patch\n*** Update File: README.md\n@@\n+note\n*** End Patch\n"}`,
		`"*** Begin Patch\n*** Update File: README.md\n@@\n+note\n*** End Patch\n"`,
	}
	for _, toolInput := range cases {
		raw := []byte(`{"session_id":"s","cwd":"/w","hook_event_name":"PostToolUse","tool_name":"functions.apply_patch","tool_input":` + toolInput + `}`)
		events, _, _ := ParseCodex(raw, now)
		if len(events) != 1 || events[0].Kind != store.KindFileTouch || events[0].Path != "README.md" || events[0].Op != "edit" {
			t.Fatalf("tool_input %s produced %+v", toolInput, events)
		}
	}
}

func TestParseCodexStop(t *testing.T) {
	events, _, _ := ParseCodex(codexFixture(t, "stop.json"), now)
	if len(events) != 1 || events[0].Kind != store.KindSessionEnd {
		t.Fatalf("events = %+v", events)
	}
}

func TestParseCodexTotalOnGarbage(t *testing.T) {
	for _, raw := range []string{"", "not json", `{"hook_event_name":"SomethingNew"}`, `{"hook_event_name":"PostToolUse","tool_name":"Bash"}`} {
		events, _, _ := ParseCodex([]byte(raw), now)
		if len(events) != 0 {
			t.Fatalf("garbage %q produced events %+v", raw, events)
		}
	}
}
