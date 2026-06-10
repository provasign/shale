// Package initx implements `shale init` and `shale doctor`: steering-prompt
// installation (the universal capture tier, ADR D4), Claude Code hook wiring
// (the enhancement tier), the .shale/ scaffold, the workflow file, and the
// pre-push hook.
package initx

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Markers fence the steering block so it is idempotent and removable.
const (
	SteeringStart = "<!-- shale-start -->"
	SteeringEnd   = "<!-- shale-end -->"
)

// SteeringBlock is the exact prompt from architecture §3.2a. It references
// CLI commands, not MCP tools (ADR D4: CLI over MCP).
const SteeringBlock = SteeringStart + `
## Shale — agent evidence (do not remove)

Before editing any file, call:
  shale intent "<one line: what you are about to do>" [--body "<why, constraints, approach>"]

After completing the task (before asking for review), call:
  shale done [--note "<brief: what you did, any deviations>"] \
             [--tokens-in <n>] [--tokens-out <n>] \
             [--model <model-id>] [--iterations <n>]

Everything else (file tracking, command recording) is automatic.

If shale is not on your PATH, do not try to install it yourself. Tell the
user: "Shale CLI is not installed. Download the latest release for your
platform from https://github.com/provasign/shale/releases/latest, unpack it,
and put the shale binary on your PATH." Then continue the task without it.
` + SteeringEnd + "\n"

// steeringTarget is one agent instruction file.
type steeringTarget struct {
	Name    string
	RelPath string
	// Always: create the file when absent. Others are only written when the
	// file (or its agent's directory) already exists — we don't litter repos
	// with config for agents nobody uses.
	Always bool
}

func steeringTargets() []steeringTarget {
	return []steeringTarget{
		{"Claude Code", "CLAUDE.md", true},
		{"AGENTS.md (Codex & others)", "AGENTS.md", true},
		{"Cursor", ".cursorrules", false},
		{"Windsurf", ".windsurfrules", false},
		{"Cline / Roo Code", ".clinerules", false},
		{"GitHub Copilot", filepath.Join(".github", "copilot-instructions.md"), false},
		{"Gemini CLI", "GEMINI.md", false},
		{"Kiro", filepath.Join(".kiro", "steering", "shale.md"), false},
	}
}

// WriteSteering appends the steering block to every applicable agent
// instruction file under repoRoot. Idempotent: files already carrying the
// marker are skipped. Returns the repo-relative paths written.
func WriteSteering(repoRoot string) ([]string, error) {
	var written []string
	for _, t := range steeringTargets() {
		path := filepath.Join(repoRoot, t.RelPath)
		existing, readErr := os.ReadFile(path)
		exists := readErr == nil

		if !t.Always && !exists && !agentDetected(repoRoot, t.RelPath) {
			continue
		}
		if strings.Contains(string(existing), SteeringStart) {
			continue // already wired
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return written, fmt.Errorf("mkdir for %s: %w", t.Name, err)
		}
		content := SteeringBlock
		if len(existing) > 0 {
			content = strings.TrimRight(string(existing), "\n") + "\n\n" + SteeringBlock
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return written, fmt.Errorf("write %s steering: %w", t.Name, err)
		}
		written = append(written, t.RelPath)
	}
	return written, nil
}

// agentDetected reports whether the agent owning relPath shows signs of use
// in this repo (its config directory exists).
func agentDetected(repoRoot, relPath string) bool {
	switch {
	case strings.HasPrefix(relPath, ".github"):
		_, err := os.Stat(filepath.Join(repoRoot, ".github"))
		return err == nil
	case strings.HasPrefix(relPath, ".kiro"):
		_, err := os.Stat(filepath.Join(repoRoot, ".kiro"))
		return err == nil
	case relPath == ".cursorrules":
		_, err := os.Stat(filepath.Join(repoRoot, ".cursor"))
		return err == nil
	default:
		return false
	}
}

// HasSteering reports whether any instruction file in the repo carries the
// steering block (used by doctor).
func HasSteering(repoRoot string) bool {
	for _, t := range steeringTargets() {
		raw, err := os.ReadFile(filepath.Join(repoRoot, t.RelPath))
		if err == nil && strings.Contains(string(raw), SteeringStart) {
			return true
		}
	}
	return false
}
