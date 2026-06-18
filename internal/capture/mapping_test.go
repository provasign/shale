package capture

import "testing"

// TestEmbeddedMapsValid guards against shipping a malformed or empty mapping:
// a bad map would make loadMapping fail and the adapter silently no-op, which
// is hard to notice. Every adapter the dispatch routes to must have a loadable
// map with at least a tool name, events, and tools.
func TestEmbeddedMapsValid(t *testing.T) {
	for _, tool := range []string{"claude-code", "codex", "cursor", "copilot"} {
		m, ok := loadMapping(tool)
		if !ok {
			t.Fatalf("map %q failed to load", tool)
		}
		if m.Tool != tool {
			t.Errorf("map %q has tool field %q", tool, m.Tool)
		}
		if len(m.Events) == 0 {
			t.Errorf("map %q has no events", tool)
		}
		if len(m.Tools) == 0 {
			t.Errorf("map %q has no tools", tool)
		}
	}
}

// TestUnknownMapMissing confirms loadMapping fails closed for an adapter with
// no embedded map (the dispatch already fails open on the same condition).
func TestUnknownMapMissing(t *testing.T) {
	if _, ok := loadMapping("nonexistent"); ok {
		t.Fatal("loadMapping returned ok for a map that does not exist")
	}
}
