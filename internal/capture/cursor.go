package capture

import (
	"encoding/json"
	"time"

	"github.com/provasign/shale/internal/store"
)

type cursorPayload struct {
	SessionID          string          `json:"session_id"`
	SessionIDCamel     string          `json:"sessionId"`
	CWD                string          `json:"cwd"`
	WorkspaceRoot      string          `json:"workspaceRoot"`
	HookEventName      string          `json:"hook_event_name"`
	HookEventNameCamel string          `json:"hookEventName"`
	Prompt             string          `json:"prompt"`
	Text               string          `json:"text"`
	ToolName           string          `json:"tool_name"`
	ToolNameCamel      string          `json:"toolName"`
	ToolInput          json.RawMessage `json:"tool_input"`
	ToolInputCamel     json.RawMessage `json:"toolInput"`
	ToolInfo           json.RawMessage `json:"tool_info"`
	ToolInfoCamel      json.RawMessage `json:"toolInfo"`
	ToolResponse       json.RawMessage `json:"tool_response"`
	ToolResponseCamel  json.RawMessage `json:"toolResponse"`
	Model              string          `json:"model"`
}

type cursorObject struct {
	FilePath      string `json:"file_path"`
	FilePathCamel string `json:"filePath"`
	Path          string `json:"path"`
	NotebookPath  string `json:"notebook_path"`
	Command       string `json:"command"`
	ExitCode      *int   `json:"exit_code"`
	ExitCodeCamel *int   `json:"exitCode"`
}

// ParseCursor normalizes one Cursor hook payload. Cursor hook payloads have
// changed over time, so this parser accepts both snake_case and camelCase
// variants and drops anything it cannot prove.
func ParseCursor(raw []byte, now time.Time) (events []store.Event, sessionID, cwd string) {
	var p cursorPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, "", ""
	}
	sessionID = firstNonEmpty(p.SessionID, p.SessionIDCamel)
	cwd = firstNonEmpty(p.CWD, p.WorkspaceRoot)
	event := firstNonEmpty(p.HookEventName, p.HookEventNameCamel)
	at := now.UTC()

	switch event {
	case "sessionStart":
		events = append(events, store.Event{
			Kind: store.KindSessionMeta, At: at,
			Tool: "cursor", Model: p.Model, SessionID: sessionID,
		})
	case "beforeSubmitPrompt":
		if prompt := firstNonEmpty(p.Prompt, p.Text); prompt != "" {
			events = append(events, store.Event{Kind: store.KindPrompt, At: at, Text: prompt})
		}
	case "afterFileEdit":
		if path := cursorPath(firstRaw(p.ToolInput, p.ToolInputCamel, p.ToolInfo, p.ToolInfoCamel, raw)); path != "" {
			events = append(events, store.Event{Kind: store.KindFileTouch, At: at, Path: path, Op: "edit"})
		}
	case "postToolUse":
		tool := firstNonEmpty(p.ToolName, p.ToolNameCamel)
		input := firstRaw(p.ToolInput, p.ToolInputCamel, p.ToolInfo, p.ToolInfoCamel)
		switch tool {
		case "Write":
			if path := cursorPath(input); path != "" {
				events = append(events, store.Event{Kind: store.KindFileTouch, At: at, Path: path, Op: "write"})
			}
		case "Edit", "MultiEdit":
			if path := cursorPath(input); path != "" {
				events = append(events, store.Event{Kind: store.KindFileTouch, At: at, Path: path, Op: "edit"})
			}
		case "Bash", "Terminal", "Shell":
			if cmd := cursorCommand(input); cmd != "" {
				resp := firstRaw(p.ToolResponse, p.ToolResponseCamel)
				events = append(events, store.Event{Kind: store.KindCommand, At: at, Cmd: cmd, ExitCode: cursorExitCode(resp)})
			}
		}
	case "sessionEnd":
		events = append(events, store.Event{Kind: store.KindSessionEnd, At: at})
	}
	return events, sessionID, cwd
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if v != "" {
			return v
		}
	}
	return ""
}

func firstRaw(vs ...json.RawMessage) json.RawMessage {
	for _, v := range vs {
		if len(v) > 0 && string(v) != "null" {
			return v
		}
	}
	return nil
}

func cursorObjectFrom(raw json.RawMessage) cursorObject {
	var obj cursorObject
	_ = json.Unmarshal(raw, &obj)
	return obj
}

func cursorPath(raw json.RawMessage) string {
	obj := cursorObjectFrom(raw)
	return firstNonEmpty(obj.FilePath, obj.FilePathCamel, obj.Path, obj.NotebookPath)
}

func cursorCommand(raw json.RawMessage) string {
	return cursorObjectFrom(raw).Command
}

func cursorExitCode(raw json.RawMessage) *int {
	obj := cursorObjectFrom(raw)
	if obj.ExitCode != nil {
		return obj.ExitCode
	}
	return obj.ExitCodeCamel
}
