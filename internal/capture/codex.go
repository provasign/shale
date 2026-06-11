package capture

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/provasign/shale/internal/store"
)

// Codex's field/event contract lives in maps/codex.json. The one piece that
// can't be a flat field map is apply_patch: edits surface as a patch envelope
// (`*** Add File:` / `*** Update File:` / `*** Delete File:` lines), and the
// tool name varies ("apply_patch", "functions.apply_patch", …). That stays in
// code here, wired in as the map's tool_fallback structural handler.
func init() {
	structuralHandlers["codex_apply_patch"] = func(toolName string, input json.RawMessage, at time.Time) []store.Event {
		if !isApplyPatchTool(toolName) {
			return nil
		}
		return codexPatchTouches(codexPatchText(input), at)
	}
}

// ParseCodex normalizes one Codex hook payload. Contract in maps/codex.json;
// zero events for unknown or incomplete payloads, preserving fail-open.
func ParseCodex(raw []byte, now time.Time) (events []store.Event, sessionID, cwd string) {
	m, ok := loadMapping("codex")
	if !ok {
		return nil, "", ""
	}
	return applyMapping(m, raw, now)
}

// codexPatchInput carries the fields a patch envelope may arrive under.
type codexPatchInput struct {
	Patch   string `json:"patch"`
	Input   string `json:"input"`
	Content string `json:"content"`
	Cmd     string `json:"cmd"`
	Command string `json:"command"`
	Text    string `json:"text"`
}

func isApplyPatchTool(name string) bool {
	return strings.Contains(strings.ToLower(name), "apply_patch")
}

func codexPatchText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}
	var in codexPatchInput
	_ = json.Unmarshal(raw, &in)
	return firstNonEmpty(in.Patch, in.Input, in.Content, in.Cmd, in.Command, in.Text)
}

func codexPatchTouches(patch string, at time.Time) []store.Event {
	var events []store.Event
	for _, line := range strings.Split(patch, "\n") {
		op, path, ok := strings.Cut(line, ": ")
		if !ok {
			continue
		}
		switch op {
		case "*** Add File":
			events = append(events, store.Event{Kind: store.KindFileTouch, At: at, Path: path, Op: "write"})
		case "*** Update File":
			events = append(events, store.Event{Kind: store.KindFileTouch, At: at, Path: path, Op: "edit"})
		case "*** Delete File":
			events = append(events, store.Event{Kind: store.KindFileTouch, At: at, Path: path, Op: "delete"})
		}
	}
	return events
}
