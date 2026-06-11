package initx

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	cmd := exec.Command("git", "init", "-b", "main")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	return root
}

func TestUninstallRoundTrip(t *testing.T) {
	root := gitRepo(t)

	// User content that must survive, and a foreign hook entry that must too.
	os.WriteFile(filepath.Join(root, "CLAUDE.md"), []byte("# My project\n\nHouse rules.\n"), 0o644)
	os.MkdirAll(filepath.Join(root, ".cursor"), 0o755)
	foreignCursor := `{"version":1,"hooks":{"postToolUse":[{"command":"my-linter --fix"}]}}`
	os.WriteFile(filepath.Join(root, ".cursor", "hooks.json"), []byte(foreignCursor), 0o644)

	if _, err := WriteSteering(root); err != nil {
		t.Fatal(err)
	}
	if _, err := InstallRepoHooks(root); err != nil {
		t.Fatal(err)
	}
	if _, err := Scaffold(root, "redacted"); err != nil {
		t.Fatal(err)
	}
	if _, err := InstallPrePushHook(root); err != nil {
		t.Fatal(err)
	}

	// Uninstall, repo scope.
	if removed, chained, err := RemovePrePushHook(root); err != nil || !removed || chained {
		t.Fatalf("pre-push: removed=%v chained=%v err=%v", removed, chained, err)
	}
	if _, err := RemoveSteering(root); err != nil {
		t.Fatal(err)
	}
	if _, err := RemoveRepoHooks(root); err != nil {
		t.Fatal(err)
	}
	if _, err := RemoveScaffold(root); err != nil {
		t.Fatal(err)
	}

	// User content survives, our block is gone.
	claude, err := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
	if err != nil || !strings.Contains(string(claude), "House rules") {
		t.Fatalf("user CLAUDE.md content lost: %s (%v)", claude, err)
	}
	if strings.Contains(string(claude), SteeringStart) {
		t.Fatal("steering block survived in CLAUDE.md")
	}
	// AGENTS.md was created by init for the block alone — must be gone.
	if _, err := os.Stat(filepath.Join(root, "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatal("AGENTS.md not removed")
	}

	// Foreign cursor entry survives, shale entries gone, file kept.
	cursor, err := os.ReadFile(filepath.Join(root, ".cursor", "hooks.json"))
	if err != nil {
		t.Fatalf("cursor hooks file deleted despite foreign entry: %v", err)
	}
	if !strings.Contains(string(cursor), "my-linter") || strings.Contains(string(cursor), "shale capture") {
		t.Fatalf("cursor hooks wrong after uninstall: %s", cursor)
	}

	// Files init created from nothing are gone entirely.
	for _, rel := range []string{
		filepath.Join(".claude", "settings.json"),
		filepath.Join(".github", "hooks", "shale.json"),
		filepath.Join(".codex", "hooks.json"),
		filepath.Join(".github", "workflows", "shale.yml"),
		".shale",
	} {
		if _, err := os.Stat(filepath.Join(root, rel)); !os.IsNotExist(err) {
			t.Errorf("%s not removed", rel)
		}
	}

	// pre-push hook gone.
	if _, err := os.Stat(filepath.Join(root, ".git", "hooks", "pre-push")); !os.IsNotExist(err) {
		t.Fatal("pre-push hook not removed")
	}
}

func TestRemovePrePushHookNeverTouchesForeignHooks(t *testing.T) {
	root := gitRepo(t)
	hookPath := filepath.Join(root, ".git", "hooks", "pre-push")
	os.MkdirAll(filepath.Dir(hookPath), 0o755)

	// Chained: a user hook that calls shale — flag it, never edit it.
	chainedHook := "#!/bin/sh\nnpx lint-staged\nshale finalize --auto-commit || true\n"
	os.WriteFile(hookPath, []byte(chainedHook), 0o755)
	removed, chained, err := RemovePrePushHook(root)
	if err != nil || removed || !chained {
		t.Fatalf("chained: removed=%v chained=%v err=%v", removed, chained, err)
	}
	if raw, _ := os.ReadFile(hookPath); string(raw) != chainedHook {
		t.Fatal("chained hook was modified")
	}

	// Fully foreign: nothing to do, nothing touched.
	foreign := "#!/bin/sh\necho mine\n"
	os.WriteFile(hookPath, []byte(foreign), 0o755)
	removed, chained, err = RemovePrePushHook(root)
	if err != nil || removed || chained {
		t.Fatalf("foreign: removed=%v chained=%v err=%v", removed, chained, err)
	}
	if raw, _ := os.ReadFile(hookPath); string(raw) != foreign {
		t.Fatal("foreign hook was modified")
	}
}

func TestRemoveGlobalHooksPreservesForeignSettings(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home) // Windows

	// A user's global Claude settings with their own hook and a setting.
	os.MkdirAll(filepath.Join(home, ".claude"), 0o755)
	settingsPath := filepath.Join(home, ".claude", "settings.json")
	os.WriteFile(settingsPath, []byte(`{"model":"opus","hooks":{"PostToolUse":[{"hooks":[{"type":"command","command":"my-formatter"}]}]}}`), 0o644)

	if _, err := InstallGlobalHooks(); err != nil {
		t.Fatal(err)
	}
	if !HasClaudeHooks(settingsPath) {
		t.Fatal("setup: global install did not wire claude hooks")
	}

	changed, err := RemoveGlobalHooks()
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) == 0 {
		t.Fatal("RemoveGlobalHooks reported nothing changed")
	}

	raw, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("global settings file deleted: %v", err)
	}
	var settings map[string]any
	if err := json.Unmarshal(raw, &settings); err != nil {
		t.Fatalf("global settings corrupted: %v", err)
	}
	if settings["model"] != "opus" {
		t.Fatal("user setting lost")
	}
	if strings.Contains(string(raw), "shale capture") {
		t.Fatal("shale hooks survived")
	}
	if !strings.Contains(string(raw), "my-formatter") {
		t.Fatal("user's own hook lost")
	}
}
