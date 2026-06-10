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
