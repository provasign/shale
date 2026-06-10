package capture

import (
	"encoding/json"
	"time"

	"github.com/provasign/shale/internal/store"
)

type copilotPayload struct {
	SessionID           string          `json:"session_id"`
	SessionIDCamel      string          `json:"sessionId"`
	ThreadID            string          `json:"thread_id"`
	ThreadIDCamel       string          `json:"threadId"`
	ConversationID      string          `json:"conversation_id"`
	ConversationIDCamel string          `json:"conversationId"`
	CWD                 string          `json:"cwd"`
	WorkspaceRoot       string          `json:"workspaceRoot"`
	WorkspaceRootSnake  string          `json:"workspace_root"`
	HookEventName       string          `json:"hook_event_name"`
	HookEventNameCamel  string          `json:"hookEventName"`
	Prompt              string          `json:"prompt"`
	Text                string          `json:"text"`
	ToolName            string          `json:"tool_name"`
	ToolNameCamel       string          `json:"toolName"`
	ToolInput           json.RawMessage `json:"tool_input"`
	ToolInputCamel      json.RawMessage `json:"toolInput"`
	ToolInfo            json.RawMessage `json:"tool_info"`
	ToolInfoCamel       json.RawMessage `json:"toolInfo"`
	ToolResponse        json.RawMessage `json:"tool_response"`
	ToolResponseCamel   json.RawMessage `json:"toolResponse"`
	Model               string          `json:"model"`
}

// ParseCopilot normalizes one Copilot hook payload. The repo-level Copilot
// hook config uses camelCase event names; this parser also accepts snake_case
// variants because VS Code / CLI surfaces may drift independently.
func ParseCopilot(raw []byte, now time.Time) (events []store.Event, sessionID, cwd string) {
	var p copilotPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, "", ""
	}
	sessionID = firstNonEmpty(p.SessionID, p.SessionIDCamel, p.ThreadID, p.ThreadIDCamel, p.ConversationID, p.ConversationIDCamel)
	cwd = firstNonEmpty(p.CWD, p.WorkspaceRoot, p.WorkspaceRootSnake)
	event := firstNonEmpty(p.HookEventName, p.HookEventNameCamel)
	at := now.UTC()

	switch event {
	case "sessionStart":
		events = append(events, store.Event{
			Kind: store.KindSessionMeta, At: at,
			Tool: "copilot", Model: p.Model, SessionID: sessionID,
		})
	case "userPromptSubmitted":
		if prompt := firstNonEmpty(p.Prompt, p.Text); prompt != "" {
			events = append(events, store.Event{Kind: store.KindPrompt, At: at, Text: prompt})
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
