package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/provasign/shale/internal/capture"
	"github.com/provasign/shale/internal/gitx"
	"github.com/provasign/shale/internal/store"
)

// cmdCapture is the hook entry point. HARD REQUIREMENTS (plan B2): finish in
// <50 ms and exit 0 on EVERY error — a Shale bug must never break the
// agent's hook chain. Errors go to .shale/local/capture.log, never stderr
// noise that could surface in the agent loop.
func cmdCapture(args []string, stdin io.Reader, stderr io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintln(stderr, "usage: shale capture <adapter>   (adapters: claude-code, codex, cursor, copilot)")
		return 0 // fail-open even on misuse
	}
	adapter := args[0]

	payload, err := io.ReadAll(io.LimitReader(stdin, 4<<20))
	if err != nil {
		return 0
	}

	var events []store.Event
	var sessionID, cwd string
	switch adapter {
	case "claude-code":
		events, sessionID, cwd = capture.ParseClaudeCode(payload, time.Now())
	case "codex":
		events, sessionID, cwd = capture.ParseCodex(payload, time.Now())
	case "cursor":
		events, sessionID, cwd = capture.ParseCursor(payload, time.Now())
	case "copilot":
		events, sessionID, cwd = capture.ParseCopilot(payload, time.Now())
	default:
		return 0 // unknown adapter: silently fail open (forward compat)
	}
	if len(events) == 0 || sessionID == "" {
		return 0
	}

	// Locate the repo root from the hook payload's cwd, not our own wd —
	// hooks may run from anywhere.
	root := gitx.Root(cwd)
	if root == "" {
		root = cwd
	}
	if root == "" {
		return 0
	}
	// Only capture into repos that opted in (have a .shale/ scaffold).
	if _, err := os.Stat(filepath.Join(root, ".shale")); err != nil {
		return 0
	}

	for _, ev := range events {
		if err := store.AppendEvent(root, sessionID, ev); err != nil {
			logCaptureError(root, err)
			return 0
		}
	}
	// Track the active session so `shale intent` / `shale done` (CLI calls
	// without a payload) land in it.
	if err := store.SetCurrentSession(root, sessionID); err != nil {
		logCaptureError(root, err)
	}
	return 0
}

func logCaptureError(root string, err error) {
	path := filepath.Join(store.LocalDir(root), "capture.log")
	f, ferr := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if ferr != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s %v\n", time.Now().UTC().Format(time.RFC3339), err)
}
