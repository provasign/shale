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
//  1. Every command is fail-open in the shell that runs it: when `shale` is
//     not on PATH it must be a SILENT no-op, not an error on every event. The
//     shell differs per agent and per OS, so each gets the right guard form
//     (§ command builders below).
//  2. We write config for ALL agents, no detection, no prompting. The person
//     running init cannot know which agents future contributors use (in open
//     source, nobody can). The files are small, namespaced, reviewable, and
//     inert until both the binary and the matching adapter exist; `shale
//     capture` fails open on unknown adapters, so wiring may safely precede
//     the adapter shipping (only claude-code is implemented in MVP 1).
//
// Detection-gating only makes sense for --global, where the question is what
// is installed on *this* machine (see InstallGlobalHooks).
//
// Per-agent shell & shape, verified against vendor docs (2026-06-10):
//   - Claude Code: single `command`, run via `sh -c` (unix) / Git Bash (win
//     default). POSIX guard works on both. Nested matcher-group shape.
//   - Codex: `command` (+ optional `commandWindows`), nested matcher-group
//     shape. Windows shell for commandWindows is unspecified by the docs;
//     we emit cmd.exe-native fail-open and will confirm by fixture when the
//     codex adapter ships (MVP 2 — inert until then).
//   - Copilot (.github/hooks, CLI + cloud agent + VS Code): FLAT handlers
//     with `bash`/`powershell` fields. A bare `command` is copied to BOTH
//     bash and powershell, so a POSIX string would break under PowerShell —
//     we emit explicit `bash` and `powershell` guards instead.
//   - Cursor: flat handler with `command`. Windows shell unverified; POSIX
//     guard for now (adapter is MVP 2 — inert until then).

// --- command builders: the same capture call, fail-open per shell -----------

// posixGuard runs the capture iff `shale` resolves, swallowing every failure
// so the hook always exits 0. Works in sh, bash and Git Bash.
func posixGuard(adapter string) string {
	return "command -v shale >/dev/null 2>&1 && shale capture " + adapter + " || true"
}

// cmdGuard is the cmd.exe equivalent: `ver >NUL` is cmd's always-0 "true".
func cmdGuard(adapter string) string {
	return "where shale >NUL 2>&1 && shale capture " + adapter + " || ver >NUL"
}

// psGuard is the PowerShell equivalent: Get-Command returns nothing when
// absent, the if-body is skipped, the script exits 0.
func psGuard(adapter string) string {
	return "if (Get-Command shale -ErrorAction SilentlyContinue) { shale capture " + adapter + " }"
}

// plain is the unguarded call for --global installs: the user just ran the
// binary on this machine, so the guard would only obscure.
func plain(adapter string) string { return "shale capture " + adapter }

// --- per-agent entry shapes -------------------------------------------------

// nestedEntry is the Claude/Codex matcher-group shape: event → [{hooks:
// [handler]}] with matcher omitted (matches all). win=="" omits commandWindows.
func nestedEntry(posix, win string) []any {
	h := map[string]any{"type": "command", "command": posix}
	if win != "" {
		h["commandWindows"] = win
	}
	return []any{map[string]any{"hooks": []any{h}}}
}

// copilotEntry is the flat Copilot handler with explicit per-shell commands.
func copilotEntry(bash, powershell string) []any {
	return []any{map[string]any{"type": "command", "bash": bash, "powershell": powershell}}
}

// cursorEntry is the flat Cursor handler.
func cursorEntry(command string) []any {
	return []any{map[string]any{"command": command}}
}

// InstallRepoHooks writes/merges the committed hook config for every agent
// into repoRoot. Idempotent and additive: foreign entries are preserved, ours
// are added once (detected by "shale capture" in any command field). Returns
// the repo-relative paths that changed.
func InstallRepoHooks(repoRoot string) ([]string, error) {
	var written []string

	// Claude Code + VS Code Copilot — one Claude-format file covers both
	// (VS Code's chat.hookFilesLocations enables .claude/settings.json by
	// default; docs/06-agent-hooks.md §2.1).
	claudeRel := filepath.Join(".claude", "settings.json")
	changed, err := installClaudeHooksAt(filepath.Join(repoRoot, claudeRel), posixGuard("claude-code"))
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
// Codex with guarded (fail-open) commands. Event names and entry shapes follow
// the official docs (docs/06-agent-hooks.md §2); the capture adapters for
// these agents land in MVP 2 — until then the configs are inert by design.
func repoHookTargets() []repoHookTarget {
	copilot := copilotEntry(posixGuard("copilot"), psGuard("copilot"))
	cursor := cursorEntry(posixGuard("cursor"))
	codex := nestedEntry(posixGuard("codex"), cmdGuard("codex"))
	return []repoHookTarget{
		{
			// Copilot CLI + Copilot cloud agent + VS Code Copilot. camelCase
			// event names (Copilot CLI native; VS Code converts).
			Name:    "Copilot",
			RelPath: filepath.Join(".github", "hooks", "shale.json"),
			Events: map[string][]any{
				"sessionStart":        copilot,
				"userPromptSubmitted": copilot,
				"postToolUse":         copilot,
				"sessionEnd":          copilot,
			},
		},
		{
			Name:    "Cursor",
			RelPath: filepath.Join(".cursor", "hooks.json"),
			Top:     map[string]any{"version": 1},
			Events: map[string][]any{
				"sessionStart":       cursor,
				"beforeSubmitPrompt": cursor,
				"postToolUse":        cursor,
				"afterFileEdit":      cursor,
				"sessionEnd":         cursor,
			},
		},
		{
			// Codex: nested matcher-group shape (like Claude), commandWindows
			// for cmd.exe. Requires a one-time `/hooks` trust review before
			// committed hooks run (docs/06-agent-hooks.md §2.3).
			Name:    "Codex",
			RelPath: filepath.Join(".codex", "hooks.json"),
			Events: map[string][]any{
				"SessionStart":     codex,
				"UserPromptSubmit": codex,
				"PostToolUse":      codex,
				"Stop":             codex,
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
// (`>`, `&&`) must stay readable, not become &gt; soup.
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
// in any entry shape (flat {"command"|"bash"|"powershell": …} or nested
// {"hooks": [{"command": …}]}).
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
// on this machine's PATH, the user just ran it — but the same per-agent shapes
// as the repo files. Returns the paths changed.
func InstallGlobalHooks() ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	var written []string

	if dirExists(filepath.Join(home, ".claude")) {
		p := filepath.Join(home, ".claude", "settings.json")
		changed, err := InstallClaudeHooks(p) // plain "shale capture claude-code"
		if err != nil {
			return written, err
		}
		if changed {
			written = append(written, p)
		}
	}

	cursor := cursorEntry(plain("cursor"))
	codex := nestedEntry(plain("codex"), plain("codex"))
	copilot := copilotEntry(plain("copilot"), plain("copilot"))
	type globalTarget struct {
		dir, path string
		top       map[string]any
		events    map[string][]any
	}
	targets := []globalTarget{
		{
			dir: filepath.Join(home, ".cursor"), path: filepath.Join(home, ".cursor", "hooks.json"),
			top: map[string]any{"version": 1},
			events: map[string][]any{
				"sessionStart": cursor, "beforeSubmitPrompt": cursor, "postToolUse": cursor,
				"afterFileEdit": cursor, "sessionEnd": cursor,
			},
		},
		{
			dir: filepath.Join(home, ".codex"), path: filepath.Join(home, ".codex", "hooks.json"),
			events: map[string][]any{
				"SessionStart": codex, "UserPromptSubmit": codex, "PostToolUse": codex, "Stop": codex,
			},
		},
		{
			dir: filepath.Join(home, ".copilot"), path: filepath.Join(home, ".copilot", "hooks", "shale.json"),
			events: map[string][]any{
				"sessionStart": copilot, "userPromptSubmitted": copilot, "postToolUse": copilot, "sessionEnd": copilot,
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
