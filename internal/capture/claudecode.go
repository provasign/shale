// Package capture turns agent hook payloads into normalized store.Events.
// Adapters are tier 2 of the capture model (ADR D4): best-effort enhancement.
// Every parser here must be total — malformed input returns (nil, "", ""),
// never an error that could break the agent's hook chain.
//
// Each adapter's contract (event names, field names, snake/camel variants,
// tool→op) lives in maps/<tool>.json and is read by the shared engine in
// mapping.go, so a vendor format change is a JSON edit, not a code change.
// Extraction that can't be a flat field map (e.g. Codex's apply_patch envelope)
// stays in code via structuralHandlers; see codex.go.
package capture

import (
	"time"

	"github.com/provasign/shale/internal/store"
)

// AdapterVersion is stamped into shale files produced from these events.
const AdapterVersion = "0.1.0"

// ParseClaudeCode normalizes one Claude Code hook payload. Contract in
// maps/claude-code.json. A payload we cannot use yields zero events — never
// an error. Both Stop (per-turn) and SessionEnd (once at termination) map to
// session_end; finalize is idempotent, so duplicates are harmless.
func ParseClaudeCode(raw []byte, now time.Time) (events []store.Event, sessionID, cwd string) {
	m, ok := loadMapping("claude-code")
	if !ok {
		return nil, "", ""
	}
	return applyMapping(m, raw, now)
}
