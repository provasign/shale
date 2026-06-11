package capture

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/provasign/shale/internal/store"
)

// codexPayload covers the Codex CLI hook stdin JSON surface Shale reads.
// Fixtures under testdata/codex/ were shaped from Codex CLI 0.139.0 hook docs
// and local hook-reader behavior; unknown fields are ignored.
type codexPayload struct {
	SessionID     string          `json:"session_id"`
	CWD           string          `json:"cwd"`
	HookEventName string          `json:"hook_event_name"`
	Prompt        string          `json:"prompt"`
	ToolName      string          `json:"tool_name"`
	ToolInput     json.RawMessage `json:"tool_input"`
	ToolResponse  json.RawMessage `json:"tool_response"`
	Model         string          `json:"model"`
}

type codexToolInput struct {
	FilePath      string `json:"file_path"`
	FilePathCamel string `json:"filePath"`
	Path          string `json:"path"`
	NotebookPath  string `json:"notebook_path"`
	Command       string `json:"command"`
	Patch         string `json:"patch"`
	Input         string `json:"input"`
	Content       string `json:"content"`
	Cmd           string `json:"cmd"`
	Text          string `json:"text"`
}

type codexToolResponse struct {
	ExitCode *int `json:"exit_code"`
}

// ParseCodex normalizes one Codex hook payload. It returns zero events for
// unknown or incomplete payloads, preserving hook fail-open behavior.
func ParseCodex(raw []byte, now time.Time) (events []store.Event, sessionID, cwd string) {
	var p codexPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, "", ""
	}
	sessionID, cwd = p.SessionID, p.CWD
	at := now.UTC()

	switch p.HookEventName {
	case "SessionStart":
		events = append(events, store.Event{
			Kind: store.KindSessionMeta, At: at,
			Tool: "codex", Model: p.Model, SessionID: p.SessionID,
		})
	case "UserPromptSubmit":
		if p.Prompt != "" {
			events = append(events, store.Event{Kind: store.KindPrompt, At: at, Text: p.Prompt})
		}
	case "PostToolUse":
		var in codexToolInput
		_ = json.Unmarshal(p.ToolInput, &in)
		switch p.ToolName {
		case "Write":
			if in.FilePath != "" {
				events = append(events, store.Event{Kind: store.KindFileTouch, At: at, Path: in.FilePath, Op: "write"})
			}
		case "Edit", "MultiEdit":
			if in.FilePath != "" {
				events = append(events, store.Event{Kind: store.KindFileTouch, At: at, Path: in.FilePath, Op: "edit"})
			}
		case "NotebookEdit":
			if in.NotebookPath != "" {
				events = append(events, store.Event{Kind: store.KindFileTouch, At: at, Path: in.NotebookPath, Op: "edit"})
			}
		case "Bash":
			if in.Command != "" {
				var resp codexToolResponse
				_ = json.Unmarshal(p.ToolResponse, &resp)
				events = append(events, store.Event{Kind: store.KindCommand, At: at, Cmd: in.Command, ExitCode: resp.ExitCode})
			}
		default:
			if isApplyPatchTool(p.ToolName) {
				events = append(events, codexPatchTouches(codexPatchText(p.ToolInput), at)...)
			}
		}
	case "Stop":
		events = append(events, store.Event{Kind: store.KindSessionEnd, At: at})
	}
	return events, sessionID, cwd
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
	var in codexToolInput
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
