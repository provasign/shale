package initx

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// ShaleAllowEntries are the Claude Code permission patterns for the
// agent-called evidence commands — and only those. Never widen this to
// `Bash(shale *)`: that would silently auto-approve uninstall, init, and
// whatever ships later. Consent-gated at init time (prompt or
// --allow-agent-commands); never written by default.
var ShaleAllowEntries = []string{
	"Bash(shale intent*)",
	"Bash(shale done*)",
	"Bash(shale note*)",
}

// InstallClaudeAllowlist additively merges the evidence-command permissions
// into the Claude-format settings JSON at path (the committed repo copy).
// Idempotent; user entries and everything else in the file are preserved.
func InstallClaudeAllowlist(path string) (changed bool, err error) {
	settings := map[string]any{}
	if raw, rerr := os.ReadFile(path); rerr == nil {
		if jerr := json.Unmarshal(raw, &settings); jerr != nil {
			return false, fmt.Errorf("existing %s is not valid JSON — fix it or remove it, then re-run init: %w", path, jerr)
		}
	}

	perms, _ := settings["permissions"].(map[string]any)
	if perms == nil {
		perms = map[string]any{}
	}
	allow, _ := perms["allow"].([]any)
	have := map[string]bool{}
	for _, a := range allow {
		if s, ok := a.(string); ok {
			have[s] = true
		}
	}
	for _, e := range ShaleAllowEntries {
		if !have[e] {
			allow = append(allow, e)
			changed = true
		}
	}
	if !changed {
		return false, nil
	}

	perms["allow"] = allow
	settings["permissions"] = perms
	out, err := marshalSettings(settings)
	if err != nil {
		return false, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	return true, os.WriteFile(path, out, 0o644)
}

// RemoveClaudeAllowlist strips exactly ShaleAllowEntries from the settings
// JSON at path; anything the user added survives. Empty containers are
// dropped so an uninstall can leave the file as if we were never there.
func RemoveClaudeAllowlist(path string) (changed bool, err error) {
	raw, rerr := os.ReadFile(path)
	if rerr != nil {
		return false, nil
	}
	settings := map[string]any{}
	if jerr := json.Unmarshal(raw, &settings); jerr != nil {
		return false, fmt.Errorf("%s is not valid JSON — remove shale entries manually: %w", path, jerr)
	}

	perms, _ := settings["permissions"].(map[string]any)
	allow, _ := perms["allow"].([]any)
	if len(allow) == 0 {
		return false, nil
	}
	ours := map[string]bool{}
	for _, e := range ShaleAllowEntries {
		ours[e] = true
	}
	var kept []any
	for _, a := range allow {
		if s, ok := a.(string); ok && ours[s] {
			changed = true
			continue
		}
		kept = append(kept, a)
	}
	if !changed {
		return false, nil
	}

	if len(kept) == 0 {
		delete(perms, "allow")
	} else {
		perms["allow"] = kept
	}
	if len(perms) == 0 {
		delete(settings, "permissions")
	}
	out, err := marshalSettings(settings)
	if err != nil {
		return false, err
	}
	return true, os.WriteFile(path, out, 0o644)
}
