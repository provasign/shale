// Package capture turns agent hook payloads into normalized store.Events.
// Adapters are tier 2 of the capture model (ADR D4): best-effort enhancement.
// Every parser here must be total — malformed input returns (nil, "", nil),
// never an error that could break the agent's hook chain.
package capture

import (
	"encoding/json"
	"time"

	"github.com/provasign/shale/internal/store"
)

// AdapterVersion is stamped into shale files produced from these events.
const AdapterVersion = "0.1.0"

// claudePayload covers the Claude Code hook stdin JSON surface we read.
// Recorded fixtures live in testdata/claude-code/; the hook API moves, so
// unknown fields are ignored and missing fields degrade gracefully.
type claudePayload struct {
	SessionID     string          `json:"session_id"`
	CWD           string          `json:"cwd"`
	HookEventName string          `json:"hook_event_name"`
	Prompt        string          `json:"prompt"`
	ToolName      string          `json:"tool_name"`
	ToolInput     json.RawMessage `json:"tool_input"`
	ToolResponse  json.RawMessage `json:"tool_response"`
	Model         string          `json:"model"`
}

type claudeToolInput struct {
	FilePath     string `json:"file_path"`
	NotebookPath string `json:"notebook_path"`
	Command      string `json:"command"`
}

type claudeToolResponse struct {
	ExitCode *int `json:"exit_code"`
}

// ParseClaudeCode normalizes one Claude Code hook payload. It returns the
// events to append, the session id, and the payload cwd (used to locate the
// repo root). A payload we cannot use yields zero events — never an error.
func ParseClaudeCode(raw []byte, now time.Time) (events []store.Event, sessionID, cwd string) {
	var p claudePayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, "", ""
	}
	sessionID, cwd = p.SessionID, p.CWD
	at := now.UTC()

	switch p.HookEventName {
	case "SessionStart":
		events = append(events, store.Event{
			Kind: store.KindSessionMeta, At: at,
			Tool: "claude-code", Model: p.Model, SessionID: p.SessionID,
		})
	case "UserPromptSubmit":
		if p.Prompt != "" {
			// Prompts feed the transcript and prompt_count ONLY — never intent
			// (spec rule 5).
			events = append(events, store.Event{Kind: store.KindPrompt, At: at, Text: p.Prompt})
		}
	case "PostToolUse":
		var in claudeToolInput
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
				var resp claudeToolResponse
				_ = json.Unmarshal(p.ToolResponse, &resp)
				// Honest provenance: exit code only when the payload carries one.
				events = append(events, store.Event{Kind: store.KindCommand, At: at, Cmd: in.Command, ExitCode: resp.ExitCode})
			}
		}
	case "Stop":
		events = append(events, store.Event{Kind: store.KindSessionEnd, At: at})
	}
	return events, sessionID, cwd
}
