package initx

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Repo-level hook wiring (docs/06-agent-hooks.md §3.3/§4.2). These files are
// committed, so they run on machines we know nothing about. Two rules follow:
//
//  1. Every command is guarded: silently inert when `shale` is not on PATH —
//     a contributor who never opted in must see zero errors, ever.
//  2. We write config for ALL agents, no detection, no prompting. The person
//     running init cannot know which agents future contributors use (in open
//     source, nobody can). The files are small, namespaced, reviewable, and
//     inert until both the binary and the matching adapter exist; `shale
//     capture` fails open on unknown adapters, so wiring may safely precede
//     the adapter shipping.
//
// Detection-gating only makes sense for --global, where the question is what
// is installed on *this* machine (see InstallGlobalHooks).

// guardedCapture is the committed hook command: POSIX guard so a missing
// binary is a silent no-op, trailing `|| true` so the guard's own failure
// exit never surfaces as a hook error in the agent.
func guardedCapture(adapter string) string {
	return "command -v shale >/dev/null 2>&1 && shale capture " + adapter + " || true"
}

// guardedCaptureWindows is the cmd.exe variant, for agents whose config
// supports a per-OS command override (Codex commandWindows).
func guardedCaptureWindows(adapter string) string {
	return "where shale >NUL 2>&1 && shale capture " + adapter
}

// InstallRepoHooks writes/merges the committed hook config for every agent
// into repoRoot. Idempotent and additive: foreign entries are preserved, ours
// are added once (detected by "shale capture" in the command). Returns the
// repo-relative paths that changed.
func InstallRepoHooks(repoRoot string) ([]string, error) {
	var written []string

	// Claude Code + VS Code Copilot — one Claude-format file covers both
	// (VS Code's chat.hookFilesLocations enables .claude/settings.json by
	// default; see docs/06-agent-hooks.md §2.1).
	claudeRel := filepath.Join(".claude", "settings.json")
	changed, err := installClaudeHooksAt(filepath.Join(repoRoot, claudeRel), guardedCapture("claude-code"))
	if err != nil {
		return written, fmt.Errorf("claude repo hooks: %w", err)
	}
	if changed {
		written = append(written, claudeRel)
	}

	for _, t := range repoHookTargets() {
		changed, err := mergeHookFile(filepath.Join(repoRoot, t.RelPath), t.Top, t.Events)
		if err != nil {
			return written, fmt.Errorf("%s repo hooks: %w", t.Name, err)
		}
		if changed {
			written = append(written, t.RelPath)
		}
	}
	return written, nil
}

// repoHookTarget is one non-Claude-format hook config file.
type repoHookTarget struct {
	Name    string
	RelPath string
	Top     map[string]any   // extra top-level fields when creating the file
	Events  map[string][]any // event name → entries to ensure present
}

// repoHookTargets returns the committed hook files for Copilot, Cursor and
// Codex. Event names and entry shapes follow the official docs as of
// 2026-06 (docs/06-agent-hooks.md §2); the capture adapters for these
// agents land in MVP 2 — until then the configs are inert by design.
func repoHookTargets() []repoHookTarget {
	copilotEntry := []any{map[string]any{"type": "command", "command": guardedCapture("copilot")}}
	codexEntry := []any{map[string]any{
		"command":        guardedCapture("codex"),
		"commandWindows": guardedCaptureWindows("codex"),
	}}
	return []repoHookTarget{
		{
			// Copilot CLI + Copilot cloud agent + VS Code Copilot. camelCase
			// event names (Copilot CLI native; VS Code converts).
			Name:    "Copilot",
			RelPath: filepath.Join(".github", "hooks", "shale.json"),
			Events: map[string][]any{
				"sessionStart":        copilotEntry,
				"userPromptSubmitted": copilotEntry,
				"postToolUse":         copilotEntry,
				"sessionEnd":          copilotEntry,
			},
		},
		{
			Name:    "Cursor",
			RelPath: filepath.Join(".cursor", "hooks.json"),
			Top:     map[string]any{"version": 1},
			Events: map[string][]any{
				"sessionStart":       {map[string]any{"command": guardedCapture("cursor")}},
				"beforeSubmitPrompt": {map[string]any{"command": guardedCapture("cursor")}},
				"postToolUse":        {map[string]any{"command": guardedCapture("cursor")}},
				"afterFileEdit":      {map[string]any{"command": guardedCapture("cursor")}},
				"sessionEnd":         {map[string]any{"command": guardedCapture("cursor")}},
			},
		},
		{
			// Codex requires a one-time `/hooks` trust review before
			// committed hooks run (docs/06-agent-hooks.md §2.3).
			Name:    "Codex",
			RelPath: filepath.Join(".codex", "hooks.json"),
			Events: map[string][]any{
				"SessionStart":     codexEntry,
				"UserPromptSubmit": codexEntry,
				"PostToolUse":      codexEntry,
				"Stop":             codexEntry,
			},
		},
	}
}

// mergeHookFile ensures the given event entries exist in the JSON hook file
// at path. Additive and idempotent: foreign entries survive, ours are
// detected by "shale capture" in any command-ish field. Invalid existing
// JSON errors out rather than being silently replaced.
func mergeHookFile(path string, top map[string]any, events map[string][]any) (changed bool, err error) {
	doc := map[string]any{}
	if raw, rerr := os.ReadFile(path); rerr == nil {
		if jerr := json.Unmarshal(raw, &doc); jerr != nil {
			return false, fmt.Errorf("existing %s is not valid JSON — fix it or remove it, then re-run init: %w", path, jerr)
		}
	}

	hooks, _ := doc["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
	}
	for event, ours := range events {
		entries, _ := hooks[event].([]any)
		if hasShaleCommand(entries) {
			continue
		}
		hooks[event] = append(entries, ours...)
		changed = true
	}
	if !changed {
		return false, nil
	}

	for k, v := range top {
		if _, exists := doc[k]; !exists {
			doc[k] = v
		}
	}
	doc["hooks"] = hooks
	out, err := marshalSettings(doc)
	if err != nil {
		return false, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	return true, os.WriteFile(path, out, 0o644)
}

// marshalSettings renders committed config JSON without HTML escaping —
// these files are human-reviewed, and the shell guards in hook commands
// (`>`, `&&`) must stay readable, not become > soup.
func marshalSettings(doc map[string]any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(doc); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// hasShaleCommand reports whether any entry carries a shale capture command,
// in either the flat ({"command": …}) or nested ({"hooks": [{"command": …}]})
// entry shape.
func hasShaleCommand(entries []any) bool {
	var scan func(v any) bool
	scan = func(v any) bool {
		switch x := v.(type) {
		case string:
			return strings.Contains(x, "shale capture")
		case map[string]any:
			for _, vv := range x {
				if scan(vv) {
					return true
				}
			}
		case []any:
			for _, vv := range x {
				if scan(vv) {
					return true
				}
			}
		}
		return false
	}
	return scan(entries)
}

// HasRepoHooks reports whether every committed hook config is wired (used by
// doctor).
func HasRepoHooks(repoRoot string) bool {
	if !HasClaudeHooks(filepath.Join(repoRoot, ".claude", "settings.json")) {
		return false
	}
	for _, t := range repoHookTargets() {
		raw, err := os.ReadFile(filepath.Join(repoRoot, t.RelPath))
		if err != nil || !strings.Contains(string(raw), "shale capture") {
			return false
		}
	}
	return true
}

// InstallGlobalHooks writes machine-wide hook config for every agent whose
// home directory exists (detection-gated: this is about what is installed on
// *this* machine, unlike the committed repo files). Plain commands — shale is
// on this machine's PATH, the user just ran it. Returns the paths changed.
func InstallGlobalHooks() ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	var written []string

	if dirExists(filepath.Join(home, ".claude")) {
		p := filepath.Join(home, ".claude", "settings.json")
		changed, err := InstallClaudeHooks(p)
		if err != nil {
			return written, err
		}
		if changed {
			written = append(written, p)
		}
	}
	type globalTarget struct {
		dir, path string
		top       map[string]any
		events    map[string][]any
	}
	plain := func(adapter string, nested bool) []any {
		if nested {
			return []any{map[string]any{"type": "command", "command": "shale capture " + adapter}}
		}
		return []any{map[string]any{"command": "shale capture " + adapter}}
	}
	targets := []globalTarget{
		{
			dir: filepath.Join(home, ".cursor"), path: filepath.Join(home, ".cursor", "hooks.json"),
			top: map[string]any{"version": 1},
			events: map[string][]any{
				"sessionStart": plain("cursor", false), "beforeSubmitPrompt": plain("cursor", false),
				"postToolUse": plain("cursor", false), "afterFileEdit": plain("cursor", false),
				"sessionEnd": plain("cursor", false),
			},
		},
		{
			dir: filepath.Join(home, ".codex"), path: filepath.Join(home, ".codex", "hooks.json"),
			events: map[string][]any{
				"SessionStart": plain("codex", false), "UserPromptSubmit": plain("codex", false),
				"PostToolUse": plain("codex", false), "Stop": plain("codex", false),
			},
		},
		{
			dir: filepath.Join(home, ".copilot"), path: filepath.Join(home, ".copilot", "hooks", "shale.json"),
			events: map[string][]any{
				"sessionStart": plain("copilot", true), "userPromptSubmitted": plain("copilot", true),
				"postToolUse": plain("copilot", true), "sessionEnd": plain("copilot", true),
			},
		},
	}
	for _, t := range targets {
		if !dirExists(t.dir) {
			continue
		}
		changed, err := mergeHookFile(t.path, t.top, t.events)
		if err != nil {
			return written, err
		}
		if changed {
			written = append(written, t.path)
		}
	}
	return written, nil
}

func dirExists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}
