package capture

import "encoding/json"

// firstNonEmpty returns the first non-empty string. Used by the codex
// apply_patch handler (codex.go).
func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if v != "" {
			return v
		}
	}
	return ""
}

// firstRaw returns the first present, non-null raw message. Used by the
// mapping engine (mapping.go).
func firstRaw(vs ...json.RawMessage) json.RawMessage {
	for _, v := range vs {
		if len(v) > 0 && string(v) != "null" {
			return v
		}
	}
	return nil
}
