package initx

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Uninstall reverses `shale init`. It mirrors init's never-destroy rule in
// the other direction: only things Shale wrote are removed — foreign hook
// entries, user instruction text, and a hook manager's pre-push file all
// survive untouched.

// RemovePrePushHook deletes the pre-push hook when it is exactly ours.
// chained=true means a non-Shale hook contains a `shale finalize` line the
// user added themselves (husky/lefthook chaining) — we never edit foreign
// hooks, so the caller tells the user to remove that line.
func RemovePrePushHook(repoRoot string) (removed, chained bool, err error) {
	path := PrePushHookPath(repoRoot)
	raw, rerr := os.ReadFile(path)
	if rerr != nil {
		return false, false, nil // nothing installed
	}
	if string(raw) == prePushScript {
		return true, false, os.Remove(path)
	}
	if strings.Contains(string(raw), "shale finalize") {
		return false, true, nil
	}
	return false, false, nil // foreign hook without shale — not ours to touch
}

// RemoveSteering cuts the marker-fenced steering block from every
// instruction file that carries it. A file left with only whitespace was
// created by init for the block alone, so it is removed; any user content
// around the block is preserved byte-for-byte.
func RemoveSteering(repoRoot string) ([]string, error) {
	var changed []string
	for _, t := range steeringTargets() {
		path := filepath.Join(repoRoot, t.RelPath)
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		content := string(raw)
		start := strings.Index(content, SteeringStart)
		if start < 0 {
			continue
		}
		end := strings.Index(content, SteeringEnd)
		if end < 0 {
			continue // half a fence is not ours to guess about
		}
		end += len(SteeringEnd)
		remaining := strings.TrimRight(content[:start], "\n") + strings.TrimPrefix(content[end:], "\n")
		if strings.TrimSpace(remaining) == "" {
			if err := os.Remove(path); err != nil {
				return changed, err
			}
		} else {
			if err := os.WriteFile(path, []byte(strings.TrimRight(remaining, "\n")+"\n"), 0o644); err != nil {
				return changed, err
			}
		}
		changed = append(changed, t.RelPath)
	}
	return changed, nil
}

// RemoveClaudeHooks filters Shale capture entries out of a Claude-format
// settings JSON (repo or global). deleteWhenEmpty removes the file when
// nothing else remains — used for the repo copy init created, never for the
// user's global settings.
func RemoveClaudeHooks(path string, deleteWhenEmpty bool) (changed bool, err error) {
	raw, rerr := os.ReadFile(path)
	if rerr != nil {
		return false, nil
	}
	settings := map[string]any{}
	if jerr := json.Unmarshal(raw, &settings); jerr != nil {
		return false, fmt.Errorf("%s is not valid JSON — remove shale entries manually: %w", path, jerr)
	}
	if filterHooksMap(settings) {
		changed = true
	}
	if !changed {
		return false, nil
	}
	if deleteWhenEmpty && len(settings) == 0 {
		return true, os.Remove(path)
	}
	out, err := marshalSettings(settings)
	if err != nil {
		return false, err
	}
	return true, os.WriteFile(path, out, 0o644)
}

// RemoveRepoHooks reverses InstallRepoHooks: Shale entries are filtered out
// of every committed agent hook config; a file holding nothing foreign is
// deleted. Returns the repo-relative paths that changed.
func RemoveRepoHooks(repoRoot string) ([]string, error) {
	var changed []string

	claudeRel := filepath.Join(".claude", "settings.json")
	did, err := RemoveClaudeHooks(filepath.Join(repoRoot, claudeRel), true)
	if err != nil {
		return changed, err
	}
	if did {
		changed = append(changed, claudeRel)
	}

	for _, t := range repoHookTargets() {
		did, err := removeFromHookFile(filepath.Join(repoRoot, t.RelPath), t.Top)
		if err != nil {
			return changed, err
		}
		if did {
			changed = append(changed, t.RelPath)
		}
	}
	return changed, nil
}

// RemoveGlobalHooks reverses `shale init --global`: Shale entries are
// filtered out of the machine-wide agent configs. The user's settings files
// are never deleted, only the standalone shale.json Copilot file may go.
func RemoveGlobalHooks() ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	var changed []string

	claudePath := filepath.Join(home, ".claude", "settings.json")
	did, err := RemoveClaudeHooks(claudePath, false)
	if err != nil {
		return changed, err
	}
	if did {
		changed = append(changed, claudePath)
	}

	for _, t := range []struct {
		path string
		top  map[string]any
	}{
		{filepath.Join(home, ".cursor", "hooks.json"), map[string]any{"version": 1}},
		{filepath.Join(home, ".codex", "hooks.json"), nil},
		{filepath.Join(home, ".copilot", "hooks", "shale.json"), nil},
	} {
		did, err := removeFromHookFile(t.path, t.top)
		if err != nil {
			return changed, err
		}
		if did {
			changed = append(changed, t.path)
		}
	}
	return changed, nil
}

// removeFromHookFile filters Shale entries out of a generic JSON hook file.
// When nothing foreign remains — top-level fields init itself added (e.g.
// Cursor's "version") don't count — the file is deleted.
func removeFromHookFile(path string, top map[string]any) (changed bool, err error) {
	raw, rerr := os.ReadFile(path)
	if rerr != nil {
		return false, nil
	}
	doc := map[string]any{}
	if jerr := json.Unmarshal(raw, &doc); jerr != nil {
		return false, fmt.Errorf("%s is not valid JSON — remove shale entries manually: %w", path, jerr)
	}
	if !filterHooksMap(doc) {
		return false, nil
	}
	foreign := false
	for k := range doc {
		if _, ours := top[k]; !ours {
			foreign = true
		}
	}
	if !foreign {
		return true, os.Remove(path)
	}
	out, err := marshalSettings(doc)
	if err != nil {
		return false, err
	}
	return true, os.WriteFile(path, out, 0o644)
}

// filterHooksMap strips Shale entries from doc["hooks"], dropping emptied
// events and the hooks key itself. Reports whether anything was removed.
func filterHooksMap(doc map[string]any) (changed bool) {
	hooks, _ := doc["hooks"].(map[string]any)
	if hooks == nil {
		return false
	}
	for event, v := range hooks {
		entries, _ := v.([]any)
		var kept []any
		for _, e := range entries {
			if hasShaleCommand([]any{e}) {
				changed = true
				continue
			}
			kept = append(kept, e)
		}
		if len(kept) == 0 {
			delete(hooks, event)
		} else {
			hooks[event] = kept
		}
	}
	if len(hooks) == 0 {
		delete(doc, "hooks")
	}
	return changed
}

// RemoveScaffold deletes the committed evidence surface: .shale/ (including
// finalized evidence — the team is opting out) and the render workflow.
// Returns the repo-relative paths removed.
func RemoveScaffold(repoRoot string) ([]string, error) {
	var removed []string
	wf := filepath.Join(".github", "workflows", "shale.yml")
	if _, err := os.Stat(filepath.Join(repoRoot, wf)); err == nil {
		if err := os.Remove(filepath.Join(repoRoot, wf)); err != nil {
			return removed, err
		}
		removed = append(removed, wf)
	}
	if _, err := os.Stat(filepath.Join(repoRoot, ".shale")); err == nil {
		if err := os.RemoveAll(filepath.Join(repoRoot, ".shale")); err != nil {
			return removed, err
		}
		removed = append(removed, ".shale/")
	}
	return removed, nil
}

// PruneEmptyDirs removes now-empty directories init may have created.
// os.Remove refuses non-empty directories, so anything holding user content
// survives; ordering is deepest-first so parents empty out.
func PruneEmptyDirs(repoRoot string) {
	for _, rel := range []string{
		filepath.Join(".kiro", "steering"), ".kiro",
		filepath.Join(".github", "hooks"), filepath.Join(".github", "workflows"), ".github",
		".claude", ".codex", ".cursor",
	} {
		_ = os.Remove(filepath.Join(repoRoot, rel))
	}
}

// RemoveLocalState wipes the laptop-only working state (.shale/local/):
// raw events, prompts, archives. Finalized committed evidence is untouched.
func RemoveLocalState(repoRoot string) (removed bool, err error) {
	dir := filepath.Join(repoRoot, ".shale", "local")
	if _, serr := os.Stat(dir); serr != nil {
		return false, nil
	}
	return true, os.RemoveAll(dir)
}
