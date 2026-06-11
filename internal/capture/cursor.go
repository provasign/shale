package capture

import (
	"time"

	"github.com/provasign/shale/internal/store"
)

// ParseCursor normalizes one Cursor hook payload. The whole contract —
// event names, field names, snake/camel variants — lives in maps/cursor.json,
// so adapting to a Cursor format change is a JSON edit, not a code change.
// See mapping.go for the engine.
func ParseCursor(raw []byte, now time.Time) (events []store.Event, sessionID, cwd string) {
	m, ok := loadMapping("cursor")
	if !ok {
		return nil, "", ""
	}
	return applyMapping(m, raw, now)
}
