package capture

import (
	"time"

	"github.com/provasign/shale/internal/store"
)

// ParseCopilot normalizes one Copilot hook payload. Contract in
// maps/copilot.json — the repo-level Copilot hook config uses camelCase event
// names, and the map also lists snake_case variants because VS Code / CLI
// surfaces may drift independently.
func ParseCopilot(raw []byte, now time.Time) (events []store.Event, sessionID, cwd string) {
	m, ok := loadMapping("copilot")
	if !ok {
		return nil, "", ""
	}
	return applyMapping(m, raw, now)
}
