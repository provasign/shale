package initx

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/provasign/shale/internal/store"
)

// Check is one doctor finding: OK or a problem with one actionable fix line.
type Check struct {
	Name string
	OK   bool
	Fix  string
}

// Doctor runs every setup check for the repo (plan C2).
func Doctor(repoRoot string) []Check {
	var checks []Check
	add := func(name string, ok bool, fix string) {
		checks = append(checks, Check{Name: name, OK: ok, Fix: fix})
	}

	add("steering prompt present", HasSteering(repoRoot),
		"run `shale init` — without the steering prompt no agent will declare intent")

	claudeOK := HasClaudeHooks(filepath.Join(repoRoot, ".claude", "settings.json"))
	if !claudeOK {
		if p, err := ClaudeSettingsPath(); err == nil {
			claudeOK = HasClaudeHooks(p)
		}
	}
	add("Claude Code capture hooks installed (repo or global)", claudeOK,
		"run `shale init` to write .claude/settings.json (or `shale init --global`)")

	add("multi-agent repo hook configs present", HasRepoHooks(repoRoot),
		"run `shale init` to write .github/hooks/shale.json, .cursor/hooks.json, .codex/hooks.json")

	_, err := os.Stat(filepath.Join(repoRoot, ".shale", "schema-version"))
	add(".shale/ scaffold present", err == nil, "run `shale init`")

	wfPath := filepath.Join(repoRoot, ".github", "workflows", "shale.yml")
	wf, err := os.ReadFile(wfPath)
	add("workflow file present", err == nil, "run `shale init` to write .github/workflows/shale.yml")
	if err == nil {
		// ADR D12: a checkout step in this workflow is the one forbidden change.
		add("workflow does NOT check out PR code", !hasCheckoutStep(string(wf)),
			"REMOVE the checkout step from shale.yml — pull_request_target + checkout of PR code is a privilege-escalation hole")
	}

	hookPath := filepath.Join(repoRoot, ".git", "hooks", "pre-push")
	raw, err := os.ReadFile(hookPath)
	hookOK := err == nil && strings.Contains(string(raw), "shale finalize")
	add("pre-push hook installed", hookOK,
		"run `shale init` (an existing non-shale pre-push hook must call `shale finalize --auto-commit` itself)")

	// Events flowing: most recent capture activity, active or archived.
	add("capture events seen", lastActivity(repoRoot) != "",
		"no session events yet — run an agent session, or check hooks with the items above")

	return checks
}

// hasCheckoutStep detects an actual `uses: actions/checkout` workflow step,
// ignoring comments (the template's own "no actions/checkout" note must not
// trip the check).
func hasCheckoutStep(workflow string) bool {
	for _, line := range strings.Split(workflow, "\n") {
		if i := strings.Index(line, "#"); i >= 0 {
			line = line[:i]
		}
		trimmed := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "-"))
		if strings.HasPrefix(trimmed, "uses:") && strings.Contains(trimmed, "actions/checkout") {
			return true
		}
	}
	return false
}

// lastActivity returns a human description of the latest capture, or "".
func lastActivity(repoRoot string) string {
	var latest time.Time
	consider := func(path string) {
		if fi, err := os.Stat(path); err == nil && fi.ModTime().After(latest) {
			latest = fi.ModTime()
		}
	}
	if ids, err := store.ListSessions(repoRoot); err == nil {
		for _, id := range ids {
			consider(store.SessionPath(repoRoot, id))
		}
	}
	if entries, err := os.ReadDir(filepath.Join(store.LocalDir(repoRoot), "archive")); err == nil {
		for _, e := range entries {
			consider(filepath.Join(store.LocalDir(repoRoot), "archive", e.Name()))
		}
	}
	if latest.IsZero() {
		return ""
	}
	return fmt.Sprintf("last capture %s", latest.Format(time.RFC3339))
}
