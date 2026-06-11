package capture

import (
	"embed"
	"encoding/json"
	"sync"
	"time"

	"github.com/provasign/shale/internal/store"
)

// A mapping describes how to read one agent's hook payload as data, so that a
// vendor renaming a field or an event is a JSON edit in maps/<tool>.json
// rather than a Go change. The mapping covers the common drift — renamed
// events, renamed fields, new tool names. Genuinely structural extraction
// (e.g. Codex's apply_patch envelope) stays in code and is referenced from a
// tool entry as {"action":"structural","handler":"<name>"}; see
// structuralHandlers.
//
// Maps are embedded so a fresh install works offline with no setup. A future
// `shale update-map` can overlay newer files on top of these defaults; the
// loader below is the single seam that would consult the overlay first.

//go:embed maps/*.json
var mapFS embed.FS

var (
	mapsMu     sync.Mutex
	loadedMaps = map[string]*Mapping{}
)

// fieldMap lists, for each semantic slot, the payload keys to try in order.
// First non-empty wins, so snake_case/camelCase variants are just more entries.
type fieldMap struct {
	SessionID    []string `json:"session_id"`
	CWD          []string `json:"cwd"`
	EventName    []string `json:"event_name"`
	Prompt       []string `json:"prompt"`
	Model        []string `json:"model"`
	ToolName     []string `json:"tool_name"`
	ToolInput    []string `json:"tool_input"`
	ToolResponse []string `json:"tool_response"`
}

// inputMap lists the keys to look for inside a tool's input/response objects.
type inputMap struct {
	FilePath []string `json:"file_path"`
	Command  []string `json:"command"`
	ExitCode []string `json:"exit_code"`
}

// actionSpec is what to do for one event name or one tool name.
type actionSpec struct {
	Action  string `json:"action"`            // session_meta|prompt|file_touch|command|session_end|tool_use|structural
	Op      string `json:"op,omitempty"`      // write|edit|delete, for file_touch
	Handler string `json:"handler,omitempty"` // structuralHandlers key, for action=structural
}

// Mapping is one agent's full data-driven contract.
type Mapping struct {
	Tool   string                `json:"tool"`
	Locate fieldMap              `json:"locate"`
	Input  inputMap              `json:"input"`
	Events map[string]actionSpec `json:"events"`
	Tools  map[string]actionSpec `json:"tools"`
	// ToolFallback applies when a tool_use event names a tool not in Tools.
	// Used for structural matchers whose tool name varies (e.g. Codex's
	// apply_patch surfaces as "apply_patch", "functions.apply_patch", …); the
	// handler inspects the tool name and self-guards.
	ToolFallback *actionSpec `json:"tool_fallback,omitempty"`
}

// structuralHandlers holds the few extractors that can't be expressed as a
// flat field map. Adapters register theirs in an init(). The handler receives
// the tool name so it can self-guard (return nil for tools it doesn't own).
var structuralHandlers = map[string]func(toolName string, input json.RawMessage, at time.Time) []store.Event{}

// loadMapping returns the embedded mapping for a tool, parsed once and cached.
func loadMapping(tool string) (*Mapping, bool) {
	mapsMu.Lock()
	defer mapsMu.Unlock()
	if m, ok := loadedMaps[tool]; ok {
		return m, true
	}
	raw, err := mapFS.ReadFile("maps/" + tool + ".json")
	if err != nil {
		return nil, false
	}
	var m Mapping
	if json.Unmarshal(raw, &m) != nil {
		return nil, false
	}
	loadedMaps[tool] = &m
	return &m, true
}

// applyMapping is the engine: parse a payload by the mapping and emit events.
// Total and fail-open — any shape it cannot read yields zero events.
func applyMapping(m *Mapping, raw []byte, now time.Time) (events []store.Event, sessionID, cwd string) {
	var top map[string]json.RawMessage
	if json.Unmarshal(raw, &top) != nil {
		return nil, "", ""
	}
	sessionID = objString(top, m.Locate.SessionID)
	cwd = objString(top, m.Locate.CWD)
	event := objString(top, m.Locate.EventName)
	spec, ok := m.Events[event]
	if !ok {
		return nil, sessionID, cwd
	}
	at := now.UTC()

	switch spec.Action {
	case "session_meta":
		events = append(events, store.Event{
			Kind: store.KindSessionMeta, At: at,
			Tool: m.Tool, Model: objString(top, m.Locate.Model), SessionID: sessionID,
		})
	case "prompt":
		if text := objString(top, m.Locate.Prompt); text != "" {
			events = append(events, store.Event{Kind: store.KindPrompt, At: at, Text: text})
		}
	case "file_touch":
		if path := resolvePath(top, m); path != "" {
			events = append(events, store.Event{Kind: store.KindFileTouch, At: at, Path: path, Op: spec.Op})
		}
	case "session_end":
		events = append(events, store.Event{Kind: store.KindSessionEnd, At: at})
	case "tool_use":
		events = append(events, applyTool(top, m, at)...)
	}
	return events, sessionID, cwd
}

// applyTool handles an event whose meaning depends on which tool ran.
func applyTool(top map[string]json.RawMessage, m *Mapping, at time.Time) []store.Event {
	name := objString(top, m.Locate.ToolName)
	tspec, ok := m.Tools[name]
	if !ok {
		if m.ToolFallback == nil {
			return nil
		}
		tspec = *m.ToolFallback
	}
	switch tspec.Action {
	case "file_touch":
		if path := resolvePath(top, m); path != "" {
			return []store.Event{{Kind: store.KindFileTouch, At: at, Path: path, Op: tspec.Op}}
		}
	case "command":
		if cmd := inputString(top, m, m.Input.Command); cmd != "" {
			return []store.Event{{Kind: store.KindCommand, At: at, Cmd: cmd, ExitCode: inputExit(top, m)}}
		}
	case "structural":
		if h := structuralHandlers[tspec.Handler]; h != nil {
			return h(name, firstRaw(rawList(top, m.Locate.ToolInput)...), at)
		}
	}
	return nil
}

// resolvePath finds a file path in the tool input object, then the payload root.
func resolvePath(top map[string]json.RawMessage, m *Mapping) string {
	for _, obj := range append(inputObjects(top, m), top) {
		if p := objString(obj, m.Input.FilePath); p != "" {
			return p
		}
	}
	return ""
}

// inputString finds a string field inside the tool input object.
func inputString(top map[string]json.RawMessage, m *Mapping, keys []string) string {
	for _, obj := range inputObjects(top, m) {
		if v := objString(obj, keys); v != "" {
			return v
		}
	}
	return ""
}

// inputExit finds an exit code in the tool response object.
func inputExit(top map[string]json.RawMessage, m *Mapping) *int {
	resp := firstRaw(rawList(top, m.Locate.ToolResponse)...)
	if resp == nil {
		return nil
	}
	obj := asObj(resp)
	for _, k := range m.Input.ExitCode {
		if r, ok := obj[k]; ok {
			var n int
			if json.Unmarshal(r, &n) == nil {
				return &n
			}
		}
	}
	return nil
}

// inputObjects returns the candidate tool-input objects named by the mapping.
func inputObjects(top map[string]json.RawMessage, m *Mapping) []map[string]json.RawMessage {
	var objs []map[string]json.RawMessage
	if ti := firstRaw(rawList(top, m.Locate.ToolInput)...); ti != nil {
		objs = append(objs, asObj(ti))
	}
	return objs
}

// objString returns the first non-empty string value among keys.
func objString(obj map[string]json.RawMessage, keys []string) string {
	for _, k := range keys {
		if r, ok := obj[k]; ok {
			var s string
			if json.Unmarshal(r, &s) == nil && s != "" {
				return s
			}
		}
	}
	return ""
}

// rawList returns the raw values present for keys, in order.
func rawList(obj map[string]json.RawMessage, keys []string) []json.RawMessage {
	var out []json.RawMessage
	for _, k := range keys {
		if r, ok := obj[k]; ok {
			out = append(out, r)
		}
	}
	return out
}

func asObj(raw json.RawMessage) map[string]json.RawMessage {
	var obj map[string]json.RawMessage
	_ = json.Unmarshal(raw, &obj)
	return obj
}
