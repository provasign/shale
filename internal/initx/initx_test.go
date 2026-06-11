package initx

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteSteeringIdempotent(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "CLAUDE.md"), []byte("# My project\n\nExisting instructions.\n"), 0o644)

	written, err := WriteSteering(root)
	if err != nil {
		t.Fatal(err)
	}
	// CLAUDE.md (existing, appended) + AGENTS.md (always-create).
	if len(written) != 2 {
		t.Fatalf("written = %v", written)
	}
	claude, _ := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
	if !strings.Contains(string(claude), "Existing instructions.") {
		t.Fatal("existing content lost")
	}
	if !strings.Contains(string(claude), "shale intent") || !strings.Contains(string(claude), "shale done") {
		t.Fatal("steering block incomplete")
	}
	if !HasSteering(root) {
		t.Fatal("HasSteering false after write")
	}

	// Second run: no-op.
	written2, err := WriteSteering(root)
	if err != nil || len(written2) != 0 {
		t.Fatalf("second run wrote %v (err %v)", written2, err)
	}
	claude2, _ := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
	if strings.Count(string(claude2), SteeringStart) != 1 {
		t.Fatal("steering block duplicated")
	}
}

func TestWriteSteeringDetectsAgents(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, ".github"), 0o755)
	os.MkdirAll(filepath.Join(root, ".cursor"), 0o755)

	written, err := WriteSteering(root)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(written, " ")
	if !strings.Contains(joined, "copilot-instructions.md") {
		t.Fatalf(".github present but no copilot steering: %v", written)
	}
	if !strings.Contains(joined, ".cursorrules") {
		t.Fatalf(".cursor present but no cursor steering: %v", written)
	}
	if strings.Contains(joined, "GEMINI.md") {
		t.Fatalf("GEMINI.md created without detection: %v", written)
	}
}

func TestInstallClaudeHooksMergesAndPreserves(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	existing := `{
  "model": "opus",
  "hooks": {
    "PostToolUse": [
      {"matcher": "Bash", "hooks": [{"type": "command", "command": "other-tool record"}]}
    ]
  }
}`
	os.WriteFile(path, []byte(existing), 0o644)

	changed, err := InstallClaudeHooks(path)
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	raw, _ := os.ReadFile(path)
	var settings map[string]any
	json.Unmarshal(raw, &settings)
	if settings["model"] != "opus" {
		t.Fatal("unrelated settings lost")
	}
	if !strings.Contains(string(raw), "other-tool record") {
		t.Fatal("pre-existing hook clobbered")
	}
	if !HasClaudeHooks(path) {
		t.Fatal("HasClaudeHooks false after install")
	}

	changed2, err := InstallClaudeHooks(path)
	if err != nil || changed2 {
		t.Fatalf("second install changed=%v err=%v", changed2, err)
	}
}

func TestInstallClaudeHooksRejectsBrokenJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	os.WriteFile(path, []byte("{broken"), 0o644)
	if _, err := InstallClaudeHooks(path); err == nil {
		t.Fatal("broken settings.json must error, not be silently replaced")
	}
}

func TestInstallRepoHooksAllAgentsGuardedIdempotent(t *testing.T) {
	root := t.TempDir()

	written, err := InstallRepoHooks(root)
	if err != nil {
		t.Fatal(err)
	}
	// All four files, no detection: the initializer can't know what agents
	// future contributors use (docs/product.md §4).
	if len(written) != 4 {
		t.Fatalf("written = %v", written)
	}
	for _, rel := range []string{
		filepath.Join(".claude", "settings.json"),
		filepath.Join(".github", "hooks", "shale.json"),
		filepath.Join(".cursor", "hooks.json"),
		filepath.Join(".codex", "hooks.json"),
	} {
		raw, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatalf("%s not written: %v", rel, err)
		}
		// Committed hooks must be self-guarding: silently inert when shale
		// is not on the contributor's PATH.
		if !strings.Contains(string(raw), "command -v shale >/dev/null 2>&1 &&") {
			t.Errorf("%s hook command is not guarded:\n%s", rel, raw)
		}
		var doc map[string]any
		if json.Unmarshal(raw, &doc) != nil {
			t.Errorf("%s is not valid JSON", rel)
		}
	}
	assertRepoHookShapes(t, root)
	if !HasRepoHooks(root) {
		t.Fatal("HasRepoHooks false after install")
	}

	// Idempotent re-run.
	written2, err := InstallRepoHooks(root)
	if err != nil || len(written2) != 0 {
		t.Fatalf("re-run wrote %v (err %v)", written2, err)
	}
}

func assertRepoHookShapes(t *testing.T, root string) {
	t.Helper()

	read := func(rel string) map[string]any {
		t.Helper()
		raw, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		var doc map[string]any
		if err := json.Unmarshal(raw, &doc); err != nil {
			t.Fatalf("parse %s: %v", rel, err)
		}
		return doc
	}

	// Copilot's repo format is flat handlers with explicit per-shell fields.
	copilot := read(filepath.Join(".github", "hooks", "shale.json"))
	postToolUse := hookEntries(t, copilot, "postToolUse")
	copilotHandler := postToolUse[0].(map[string]any)
	if _, ok := copilotHandler["command"]; ok {
		t.Fatalf("copilot repo hook must not use cross-platform command fallback: %#v", copilotHandler)
	}
	if bash, _ := copilotHandler["bash"].(string); !strings.Contains(bash, "command -v shale") || !strings.Contains(bash, "|| true") {
		t.Fatalf("copilot bash guard is not fail-open: %#v", copilotHandler)
	}
	if ps, _ := copilotHandler["powershell"].(string); !strings.Contains(ps, "Get-Command shale") {
		t.Fatalf("copilot powershell guard missing: %#v", copilotHandler)
	}

	// Codex's documented JSON shape is event -> matcher group -> hooks -> command handler.
	codex := read(filepath.Join(".codex", "hooks.json"))
	codexGroups := hookEntries(t, codex, "PostToolUse")
	codexGroup := codexGroups[0].(map[string]any)
	handlers, ok := codexGroup["hooks"].([]any)
	if !ok || len(handlers) != 1 {
		t.Fatalf("codex hook must use nested matcher-group shape: %#v", codexGroup)
	}
	codexHandler := handlers[0].(map[string]any)
	if command, _ := codexHandler["command"].(string); !strings.Contains(command, "command -v shale") || !strings.Contains(command, "|| true") {
		t.Fatalf("codex POSIX guard is not fail-open: %#v", codexHandler)
	}
	if win, _ := codexHandler["commandWindows"].(string); !strings.Contains(win, "where shale") || !strings.Contains(win, "|| ver >NUL") {
		t.Fatalf("codex Windows guard is not fail-open: %#v", codexHandler)
	}
}

func hookEntries(t *testing.T, doc map[string]any, event string) []any {
	t.Helper()
	hooks, ok := doc["hooks"].(map[string]any)
	if !ok {
		t.Fatalf("missing hooks object: %#v", doc)
	}
	entries, ok := hooks[event].([]any)
	if !ok || len(entries) == 0 {
		t.Fatalf("missing hook entries for %s: %#v", event, hooks)
	}
	return entries
}

func TestInstallRepoHooksPreservesForeignEntries(t *testing.T) {
	root := t.TempDir()
	cursorPath := filepath.Join(root, ".cursor", "hooks.json")
	os.MkdirAll(filepath.Dir(cursorPath), 0o755)
	existing := `{
  "version": 1,
  "hooks": {
    "postToolUse": [{"command": "other-tool audit"}]
  }
}`
	os.WriteFile(cursorPath, []byte(existing), 0o644)

	if _, err := InstallRepoHooks(root); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(cursorPath)
	if !strings.Contains(string(raw), "other-tool audit") {
		t.Fatal("foreign cursor hook clobbered")
	}
	if !strings.Contains(string(raw), "shale capture cursor") {
		t.Fatal("shale hook not merged alongside foreign entry")
	}
	if v, _ := func() (any, error) {
		var doc map[string]any
		err := json.Unmarshal(raw, &doc)
		return doc["version"], err
	}(); v != float64(1) {
		t.Fatal("version field lost in merge")
	}
}

func TestInstallRepoHooksRejectsBrokenJSON(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, ".codex", "hooks.json")
	os.MkdirAll(filepath.Dir(p), 0o755)
	os.WriteFile(p, []byte("{broken"), 0o644)
	if _, err := InstallRepoHooks(root); err == nil {
		t.Fatal("broken hooks.json must error, not be silently replaced")
	}
}

func TestInstallGlobalHooksDetectionGated(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)        // os.UserHomeDir on unix
	t.Setenv("USERPROFILE", home) // os.UserHomeDir on Windows

	// No agent dirs: nothing written.
	written, err := InstallGlobalHooks()
	if err != nil || len(written) != 0 {
		t.Fatalf("no agents: written=%v err=%v", written, err)
	}

	// Only Cursor present: only Cursor wired, with the PLAIN command (this
	// machine has shale — the user just ran it).
	os.MkdirAll(filepath.Join(home, ".cursor"), 0o755)
	written, err = InstallGlobalHooks()
	if err != nil || len(written) != 1 {
		t.Fatalf("cursor only: written=%v err=%v", written, err)
	}
	raw, _ := os.ReadFile(filepath.Join(home, ".cursor", "hooks.json"))
	if !strings.Contains(string(raw), `"shale capture cursor"`) {
		t.Fatalf("global cursor hook should use the plain command:\n%s", raw)
	}
	if _, err := os.Stat(filepath.Join(home, ".codex", "hooks.json")); err == nil {
		t.Fatal("codex config written without ~/.codex present")
	}
}

func TestScaffoldIdempotentAndPreserving(t *testing.T) {
	root := t.TempDir()
	written, err := Scaffold(root, "redacted")
	if err != nil {
		t.Fatal(err)
	}
	if len(written) != 5 {
		t.Fatalf("written = %v", written)
	}
	wf, _ := os.ReadFile(filepath.Join(root, ".github", "workflows", "shale.yml"))
	if !strings.Contains(string(wf), "pull_request_target") {
		t.Fatal("workflow must use pull_request_target (ADR D12)")
	}
	if hasCheckoutStep(string(wf)) {
		t.Fatal("workflow must NEVER contain a checkout step (ADR D12)")
	}

	// User edits config; re-run must not clobber.
	cfgPath := filepath.Join(root, ".shale", "config.yaml")
	os.WriteFile(cfgPath, []byte("privacy: hash-only\n"), 0o644)
	written2, err := Scaffold(root, "redacted")
	if err != nil || len(written2) != 0 {
		t.Fatalf("re-scaffold wrote %v (err %v)", written2, err)
	}
	if LoadConfig(root).Privacy != "hash-only" {
		t.Fatal("user config clobbered")
	}
}

func TestLoadConfigDefaultsAndValidates(t *testing.T) {
	root := t.TempDir()
	if LoadConfig(root).Privacy != "redacted" {
		t.Fatal("default privacy must be redacted")
	}
	os.MkdirAll(filepath.Join(root, ".shale"), 0o755)
	os.WriteFile(filepath.Join(root, ".shale", "config.yaml"), []byte("privacy: shouty\n"), 0o644)
	if LoadConfig(root).Privacy != "redacted" {
		t.Fatal("invalid privacy must fall back to redacted")
	}
}

func TestInstallPrePushHook(t *testing.T) {
	root := t.TempDir()
	cmd := exec.Command("git", "init", "-b", "main")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}

	skipped, err := InstallPrePushHook(root)
	if err != nil || skipped {
		t.Fatalf("skipped=%v err=%v", skipped, err)
	}
	raw, err := os.ReadFile(filepath.Join(root, ".git", "hooks", "pre-push"))
	if err != nil || !strings.Contains(string(raw), "shale finalize --auto-commit") {
		t.Fatalf("hook content: %s (%v)", raw, err)
	}

	// Idempotent re-run.
	skipped, err = InstallPrePushHook(root)
	if err != nil || skipped {
		t.Fatalf("re-run skipped=%v err=%v", skipped, err)
	}

	// Foreign hook is never clobbered.
	foreign := "#!/bin/sh\necho mine\n"
	os.WriteFile(filepath.Join(root, ".git", "hooks", "pre-push"), []byte(foreign), 0o755)
	skipped, err = InstallPrePushHook(root)
	if err != nil || !skipped {
		t.Fatalf("foreign hook: skipped=%v err=%v", skipped, err)
	}
	raw, _ = os.ReadFile(filepath.Join(root, ".git", "hooks", "pre-push"))
	if string(raw) != foreign {
		t.Fatal("foreign hook clobbered")
	}
}

func TestInstallPrePushHookHonorsHooksPath(t *testing.T) {
	root := t.TempDir()
	for _, args := range [][]string{
		{"init", "-b", "main"},
		{"config", "core.hooksPath", ".husky"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	skipped, err := InstallPrePushHook(root)
	if err != nil || skipped {
		t.Fatalf("skipped=%v err=%v", skipped, err)
	}
	// Writing to .git/hooks here would produce a hook git never runs.
	if _, err := os.Stat(filepath.Join(root, ".git", "hooks", "pre-push")); !os.IsNotExist(err) {
		t.Fatal("hook written to .git/hooks despite core.hooksPath override")
	}
	raw, err := os.ReadFile(filepath.Join(root, ".husky", "pre-push"))
	if err != nil || !strings.Contains(string(raw), "shale finalize --auto-commit") {
		t.Fatalf("hook at core.hooksPath: %s (%v)", raw, err)
	}

	// A manager-owned hook in the override dir is never clobbered.
	foreign := "#!/bin/sh\nnpx lint-staged\n"
	os.WriteFile(filepath.Join(root, ".husky", "pre-push"), []byte(foreign), 0o755)
	if skipped, err = InstallPrePushHook(root); err != nil || !skipped {
		t.Fatalf("foreign manager hook: skipped=%v err=%v", skipped, err)
	}

	// Doctor must diagnose the same location install used.
	hp := PrePushHookPath(root)
	if filepath.Base(filepath.Dir(hp)) != ".husky" {
		t.Fatalf("PrePushHookPath = %q, want under .husky", hp)
	}
}

func TestInstallPrePushHookInLinkedWorktree(t *testing.T) {
	main := t.TempDir()
	for _, args := range [][]string{
		{"init", "-b", "main"},
		{"config", "user.email", "t@e.com"},
		{"config", "user.name", "T"},
		{"config", "commit.gpgsign", "false"},
		{"commit", "--allow-empty", "-m", "init"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = main
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	wt := filepath.Join(t.TempDir(), "wt")
	cmd := exec.Command("git", "worktree", "add", wt, "-b", "feature")
	cmd.Dir = main
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git worktree add: %v\n%s", err, out)
	}

	// In a linked worktree .git is a file; hooks live in the main repo and
	// are shared. Install must land there, not error on .git/hooks.
	skipped, err := InstallPrePushHook(wt)
	if err != nil || skipped {
		t.Fatalf("worktree install: skipped=%v err=%v", skipped, err)
	}
	raw, err := os.ReadFile(filepath.Join(main, ".git", "hooks", "pre-push"))
	if err != nil || !strings.Contains(string(raw), "shale finalize --auto-commit") {
		t.Fatalf("hook not in main repo hooks dir: %s (%v)", raw, err)
	}
}

func TestDoctor(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", t.TempDir()) // isolate ClaudeSettingsPath

	// Fresh dir: everything fails but nothing panics, and every problem has
	// an actionable fix line.
	for _, c := range Doctor(root) {
		if !c.OK && c.Fix == "" {
			t.Errorf("check %q has no fix line", c.Name)
		}
	}

	// After init-equivalent setup most checks pass.
	cmd := exec.Command("git", "init", "-b", "main")
	cmd.Dir = root
	cmd.Run()
	WriteSteering(root)
	InstallRepoHooks(root)
	Scaffold(root, "redacted")
	InstallPrePushHook(root)

	results := map[string]bool{}
	for _, c := range Doctor(root) {
		results[c.Name] = c.OK
	}
	for _, name := range []string{
		"steering prompt present", ".shale/ scaffold present",
		"workflow file present", "workflow does NOT check out PR code",
		"pre-push hook installed",
		"Claude Code capture hooks installed (repo or global)",
		"multi-agent repo hook configs present",
	} {
		if !results[name] {
			t.Errorf("check %q failed after full setup", name)
		}
	}

	// Inject the forbidden checkout step; doctor must scream.
	wf := filepath.Join(root, ".github", "workflows", "shale.yml")
	raw, _ := os.ReadFile(wf)
	os.WriteFile(wf, append(raw, []byte("      - uses: actions/checkout@v4\n")...), 0o644)
	for _, c := range Doctor(root) {
		if c.Name == "workflow does NOT check out PR code" && c.OK {
			t.Fatal("doctor missed the forbidden checkout step (ADR D12)")
		}
	}
}
