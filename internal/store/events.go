package store

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Event kinds in the session JSONL (.shale/local/<session>.jsonl).
const (
	KindIntent      = "intent"       // from `shale intent` — agent-declared, never inferred
	KindFileTouch   = "file_touch"   // from a hook adapter
	KindCommand     = "command"      // from a hook adapter
	KindSessionMeta = "session_meta" // from a hook adapter
	KindSessionEnd  = "session_end"  // from a hook adapter: a turn or the session ended
	// KindSessionClose is the agent's session ending for good (Claude Code's
	// SessionEnd, not its per-turn Stop): it closes a task the agent never
	// reported done.
	KindSessionClose = "session_close"
	KindCompletion   = "completion" // from `shale done`
	KindPrompt       = "prompt"     // raw user prompt — transcript only, NEVER intent
	KindNote         = "note"       // from `shale note`
)

// Resumed reports whether the session went on after events[i]. Claude Code
// fires SessionEnd when a conversation exits, and --resume or --continue
// brings the same session back (each turn of `claude -p --resume` is one):
// a close followed by more of the session's events ended nothing. Treated
// as an end, it closed the request, and the resumed work's shale done
// became a second task with no request (a developer trial, 2026-11-03).
func Resumed(events []Event, i int) bool {
	for _, ev := range events[i+1:] {
		if ev.Kind != KindSessionEnd && ev.Kind != KindSessionClose {
			return true
		}
	}
	return false
}

// Event is one line of the session JSONL. Fields are a union across kinds.
type Event struct {
	Kind string    `json:"kind"`
	At   time.Time `json:"at"`

	// intent
	Title string `json:"title,omitempty"`
	Body  string `json:"body,omitempty"`

	// file_touch
	Path string `json:"path,omitempty"`
	Op   string `json:"op,omitempty"`

	// command
	Cmd      string `json:"cmd,omitempty"`
	ExitCode *int   `json:"exit_code,omitempty"`

	// session_meta
	Tool        string `json:"tool,omitempty"`
	ToolVersion string `json:"tool_version,omitempty"`
	Model       string `json:"model,omitempty"`
	SessionID   string `json:"session_id,omitempty"`

	// completion
	Note       string `json:"note,omitempty"`
	TokensIn   int    `json:"tokens_in,omitempty"`
	TokensOut  int    `json:"tokens_out,omitempty"`
	Iterations int    `json:"iterations,omitempty"`

	// prompt / note
	Text string `json:"text,omitempty"`
}

// LocalDir returns .shale/local under the repo root.
func LocalDir(repoRoot string) string {
	return filepath.Join(repoRoot, ".shale", "local")
}

// SessionPath returns the JSONL path for a session id.
func SessionPath(repoRoot, sessionID string) string {
	return filepath.Join(LocalDir(repoRoot), sessionID+".jsonl")
}

// AppendEvent appends one event line to the session JSONL, creating
// directories and the file as needed.
func AppendEvent(repoRoot, sessionID string, ev Event) error {
	if sessionID == "" {
		return errors.New("session id required")
	}
	dir := LocalDir(repoRoot)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	line, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(SessionPath(repoRoot, sessionID), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(append(line, '\n')); err != nil {
		return err
	}
	return f.Sync()
}

// ReadEvents parses a session JSONL file. Malformed lines are skipped, not
// fatal — capture is fail-open and a torn write must not lose the session.
func ReadEvents(path string) ([]Event, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var events []Event
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var ev Event
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			continue
		}
		events = append(events, ev)
	}
	return events, sc.Err()
}

// CurrentSession returns the active session id recorded in
// .shale/local/current, or "" if none.
func CurrentSession(repoRoot string) string {
	raw, err := os.ReadFile(filepath.Join(LocalDir(repoRoot), "current"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}

// SetCurrentSession records the active session id so that `shale intent` /
// `shale done` (CLI calls with no hook payload) land in the right session.
func SetCurrentSession(repoRoot, sessionID string) error {
	dir := LocalDir(repoRoot)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "current"), []byte(sessionID+"\n"), 0o644)
}

// ListSessions returns the session ids with a JSONL file in .shale/local/.
func ListSessions(repoRoot string) ([]string, error) {
	entries, err := os.ReadDir(LocalDir(repoRoot))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var ids []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		ids = append(ids, strings.TrimSuffix(name, ".jsonl"))
	}
	return ids, nil
}

// ArchiveSession moves a folded session JSONL out of the active set so a
// re-run of finalize is a no-op (idempotency). When the session id was
// archived before (hook adapters recreate the JSONL when events arrive after
// a finalize), the new events are appended — the archive is the local audit
// trail and must never be clobbered.
func ArchiveSession(repoRoot, sessionID string) error {
	dir := filepath.Join(LocalDir(repoRoot), "archive")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	src := SessionPath(repoRoot, sessionID)
	dst := filepath.Join(dir, sessionID+".jsonl")
	if _, err := os.Stat(dst); err == nil {
		if err := appendFile(dst, src); err != nil {
			return fmt.Errorf("archive session %s: %w", sessionID, err)
		}
	} else if err := os.Rename(src, dst); err != nil {
		return fmt.Errorf("archive session %s: %w", sessionID, err)
	}
	cur := filepath.Join(LocalDir(repoRoot), "current")
	if raw, err := os.ReadFile(cur); err == nil && strings.TrimSpace(string(raw)) == sessionID {
		_ = os.Remove(cur) // best-effort: a stale pointer is harmless
	}
	return nil
}

// appendFile appends src's content to dst and removes src. JSONL files
// concatenate cleanly line-by-line.
func appendFile(dst, src string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(dst, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Remove(src)
}

// ArchivePart archives the folded part of a session's log and leaves the
// rest as the log: a task still in progress keeps collecting events, and
// is folded once it is closed.
func ArchivePart(repoRoot, sessionID string, folded, rest []Event) error {
	dir := filepath.Join(LocalDir(repoRoot), "archive")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	lines := func(evs []Event) ([]byte, error) {
		var b []byte
		for _, ev := range evs {
			line, err := json.Marshal(ev)
			if err != nil {
				return nil, err
			}
			b = append(append(b, line...), '\n')
		}
		return b, nil
	}
	done, err := lines(folded)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(dir, sessionID+".jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(done); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	open, err := lines(rest)
	if err != nil {
		return err
	}
	tmp := SessionPath(repoRoot, sessionID) + ".tmp"
	if err := os.WriteFile(tmp, open, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, SessionPath(repoRoot, sessionID))
}
