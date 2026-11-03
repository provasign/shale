package initx

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/provasign/shale/internal/gitx"
)

// claudeHookEvents are the Claude Code hook events Shale captures and the
// matcher each needs (empty = all). Stop fires per turn; SessionEnd fires
// once at termination — both map to session-end evidence (finalize is
// idempotent, so the duplication is harmless and the coverage is better).
var claudeHookEvents = []struct {
	Event   string
	Matcher string
}{
	{"SessionStart", ""},
	{"UserPromptSubmit", ""},
	{"PostToolUse", "Edit|Write|MultiEdit|NotebookEdit|Bash"},
	{"Stop", ""},
	{"SessionEnd", ""},
}

// ClaudeSettingsPath returns the global Claude Code settings file (hooks are
// global per-machine — docs/product.md §5).
func ClaudeSettingsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude", "settings.json"), nil
}

// InstallClaudeHooks merges Shale capture hooks into the Claude Code
// settings JSON at path (machine-global install — plain command, shale is on
// this machine's PATH). The merge is additive and idempotent: existing hooks
// (Shale's or anyone else's) are preserved; ours are added once.
func InstallClaudeHooks(path string) (changed bool, err error) {
	return installClaudeHooksAt(path, "shale capture claude-code")
}

// installClaudeHooksAt merges Shale capture hooks (with the given command)
// into the Claude-format settings JSON at path. Used for both the global
// install (plain command) and the committed repo install (guarded command).
func installClaudeHooksAt(path, command string) (changed bool, err error) {
	settings := map[string]any{}
	if raw, rerr := os.ReadFile(path); rerr == nil {
		if jerr := json.Unmarshal(raw, &settings); jerr != nil {
			return false, fmt.Errorf("existing %s is not valid JSON — fix it or remove it, then re-run init: %w", path, jerr)
		}
	}

	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
	}

	for _, he := range claudeHookEvents {
		entries, _ := hooks[he.Event].([]any)
		if hasShaleHook(entries, "shale capture claude-code") {
			continue
		}
		entry := map[string]any{
			"hooks": []any{map[string]any{"type": "command", "command": command}},
		}
		if he.Matcher != "" {
			entry["matcher"] = he.Matcher
		}
		hooks[he.Event] = append(entries, entry)
		changed = true
	}
	if !changed {
		return false, nil
	}

	settings["hooks"] = hooks
	out, err := marshalSettings(settings)
	if err != nil {
		return false, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	return true, os.WriteFile(path, out, 0o644)
}

// HasClaudeHooks reports whether the settings file already carries the
// Shale capture hooks (used by doctor).
func HasClaudeHooks(path string) bool {
	raw, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var settings struct {
		Hooks map[string]json.RawMessage `json:"hooks"`
	}
	if json.Unmarshal(raw, &settings) != nil {
		return false
	}
	for _, he := range claudeHookEvents {
		raw, ok := settings.Hooks[he.Event]
		if !ok {
			return false
		}
		var entries []any
		if json.Unmarshal(raw, &entries) != nil || !hasShaleHook(entries, "shale capture claude-code") {
			return false
		}
	}
	return true
}

// hasShaleHook detects our hook by substring, so the plain global command and
// the guarded committed variant both count as "already wired".
func hasShaleHook(entries []any, command string) bool {
	for _, e := range entries {
		entry, ok := e.(map[string]any)
		if !ok {
			continue
		}
		inner, _ := entry["hooks"].([]any)
		for _, h := range inner {
			hm, ok := h.(map[string]any)
			if !ok {
				continue
			}
			if cmd, ok := hm["command"].(string); ok && strings.Contains(cmd, command) {
				return true
			}
		}
	}
	return false
}

// prePushScript is the pre-push hook. It never commits, so nothing is
// created while the push is in flight (git sends such a commit, or leaves it
// behind, as it pleases), and never blocks the push (fail-open, ADR D5): it
// says which sessions the push does not carry yet. The evidence is committed
// with the code, by shale done and the pre-commit hook.
const prePushScript = `#!/bin/sh
# shale pre-push hook (managed by 'shale init')
command -v shale >/dev/null 2>&1 && shale prepush
exit 0
`

// oldPrePushScripts are pre-push hooks earlier releases wrote, replaced on
// init as ours.
var oldPrePushScripts = []string{`#!/bin/sh
# shale pre-push hook (managed by 'shale init')
shale finalize --auto-commit || echo "shale: finalize failed (push continues)" >&2
exit 0
`}

// preCommitScript is the pre-commit hook: the commit carries the evidence of
// the tasks closed since the last one. Never blocks the commit.
const preCommitScript = `#!/bin/sh
# shale pre-commit hook (managed by 'shale init')
command -v shale >/dev/null 2>&1 && shale precommit
exit 0
`

// preCommitLine is what init adds to a pre-commit hook it did not write
// (Stele's, a linter's), right after the shebang, so it runs before
// anything there that could end the script.
const preCommitLine = "{ command -v shale >/dev/null 2>&1 && shale precommit; } || true # shale (shale init)"

// PreCommitHookPath is where this repo's pre-commit hook lives (see
// PrePushHookPath).
func PreCommitHookPath(repoRoot string) string {
	if dir := gitx.HooksDir(repoRoot); dir != "" {
		return filepath.Join(dir, "pre-commit")
	}
	return filepath.Join(repoRoot, ".git", "hooks", "pre-commit")
}

// InstallPreCommitHook writes the pre-commit hook, or adds Shale's line to
// one that is already there: unlike pre-push, a pre-commit hook is commonly
// another tool's, and the evidence must ride the user's commit either way.
// Reports whether it added to an existing hook.
func InstallPreCommitHook(repoRoot string) (chained bool, err error) {
	if _, err := os.Stat(filepath.Join(repoRoot, ".git")); err != nil {
		return false, nil
	}
	path := PreCommitHookPath(repoRoot)
	raw, rerr := os.ReadFile(path)
	if rerr != nil {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return false, err
		}
		return false, os.WriteFile(path, []byte(preCommitScript), 0o755)
	}
	text := string(raw)
	if strings.Contains(text, "shale precommit") {
		return false, nil // ours, or already added
	}
	lines := strings.SplitAfter(text, "\n")
	at := 0
	if len(lines) > 0 && strings.HasPrefix(lines[0], "#!") {
		at = 1
	}
	out := strings.Join(lines[:at], "")
	if at == 1 && !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	out += preCommitLine + "\n" + strings.Join(lines[at:], "")
	return true, os.WriteFile(path, []byte(out), 0o755)
}

// PrePushHookPath returns where this repo's pre-push hook actually lives:
// git's effective hooks directory, which accounts for core.hooksPath
// overrides (husky/lefthook) and linked worktrees. Shared with doctor so
// install and diagnosis can never disagree about the location.
func PrePushHookPath(repoRoot string) string {
	if dir := gitx.HooksDir(repoRoot); dir != "" {
		return filepath.Join(dir, "pre-push")
	}
	return filepath.Join(repoRoot, ".git", "hooks", "pre-push")
}

// InstallPrePushHook writes the pre-push hook into the repo's effective
// hooks directory (honoring core.hooksPath). An existing hook that is not
// ours is left untouched (returned as skipped=true) — we never clobber
// user hooks.
func InstallPrePushHook(repoRoot string) (skipped bool, err error) {
	if _, err := os.Stat(filepath.Join(repoRoot, ".git")); err != nil {
		return true, nil // not a git repo (yet) — doctor will flag it
	}
	path := PrePushHookPath(repoRoot)
	if raw, rerr := os.ReadFile(path); rerr == nil {
		if string(raw) == prePushScript {
			return false, nil // already ours
		}
		old := false
		for _, s := range oldPrePushScripts {
			old = old || string(raw) == s
		}
		if !old {
			return true, nil // someone else's hook — never clobber
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	return false, os.WriteFile(path, []byte(prePushScript), 0o755)
}
