// Package gitx wraps the handful of git invocations Shale needs on the
// laptop. Everything here is best-effort: a missing or broken git must
// degrade evidence quality, never break capture or finalize (ADR D5).
package gitx

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func run(root string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &bytes.Buffer{}
	err := cmd.Run()
	return strings.TrimRight(out.String(), "\r\n"), err
}

// Root returns the repository top-level for dir, or "" when not in a repo.
func Root(dir string) string {
	out, err := run(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return ""
	}
	return filepath.Clean(out)
}

// Branch returns the current branch name, or "" when detached/unavailable.
func Branch(root string) string {
	out, err := run(root, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil || out == "HEAD" {
		return ""
	}
	return out
}

// FileChange is one path changed in the session window, with ops in the spec
// vocabulary (write | edit | delete) derived from git's status letters.
type FileChange struct {
	Path string
	Ops  []string
}

// FilesChangedSince returns repo-relative paths changed in the window from t
// to now: commits authored since t on the current branch plus any
// uncommitted (staged or unstaged) changes. This feeds the `via: git`
// fallback (ADR D4 tier 3) — honest about being a window, not a touch log.
func FilesChangedSince(root string, t time.Time) []FileChange {
	ops := map[string][]string{}
	add := func(path, op string) {
		path = strings.Trim(strings.TrimSpace(path), `"`)
		if path == "" || op == "" {
			return
		}
		for _, o := range ops[path] {
			if o == op {
				return
			}
		}
		ops[path] = append(ops[path], op)
	}

	logOut, err := run(root,
		"log", "--since="+t.UTC().Format(time.RFC3339),
		"--name-status", "--pretty=format:", "HEAD")
	if err == nil {
		for _, line := range strings.Split(logOut, "\n") {
			fields := strings.Split(line, "\t")
			if len(fields) < 2 || fields[0] == "" {
				continue
			}
			// Renames/copies are "R100\told\tnew"; the new path is the evidence.
			op := opForStatus(fields[0][0])
			if op == "" {
				op = "edit" // unknown letter still means the path changed
			}
			add(fields[len(fields)-1], op)
		}
	}

	statusOut, err := run(root, "status", "--porcelain")
	if err == nil {
		for _, line := range strings.Split(statusOut, "\n") {
			if len(line) < 4 {
				continue
			}
			path := strings.TrimSpace(line[3:])
			// Renames are reported as "old -> new"; the new path is the evidence.
			if i := strings.Index(path, " -> "); i >= 0 {
				path = path[i+4:]
			}
			// XY status: X is the index side, Y the worktree side.
			add(path, opForStatus(line[0]))
			add(path, opForStatus(line[1]))
		}
	}

	changes := make([]FileChange, 0, len(ops))
	for p, o := range ops {
		// Evidence about the evidence directory is noise.
		if p == ".shale" || strings.HasPrefix(p, ".shale/") {
			continue
		}
		changes = append(changes, FileChange{Path: p, Ops: o})
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].Path < changes[j].Path })
	return changes
}

// opForStatus maps one git status letter (diff --name-status or porcelain XY)
// to the spec ops vocabulary. Letters that carry no change ("space", ignored)
// map to "".
func opForStatus(c byte) string {
	switch c {
	case 'A', 'R', 'C', '?': // new content at this path
		return "write"
	case 'D':
		return "delete"
	case 'M', 'T', 'U':
		return "edit"
	}
	return ""
}

// HooksDir returns the directory git will actually run hooks from for this
// repo, or "" when git is unavailable so callers fall back to .git/hooks.
// rev-parse --git-path hooks honors both core.hooksPath overrides (husky,
// lefthook) and linked worktrees (where .git is a file and hooks live in
// the main repository) — a hook written to a literal .git/hooks in either
// case would never run.
func HooksDir(root string) string {
	out, err := run(root, "rev-parse", "--git-path", "hooks")
	if err != nil || out == "" {
		return ""
	}
	if !filepath.IsAbs(out) {
		out = filepath.Join(root, out)
	}
	return filepath.Clean(out)
}

// IgnoredPaths returns the subset of paths that git ignores in this repo.
// Ignored files (.env, credentials, key material) are exactly where secrets
// live, so finalize drops them from hook evidence. Best-effort: on any git
// failure this returns nil and callers must keep everything — degraded
// evidence beats broken finalize (ADR D5).
func IgnoredPaths(root string, paths []string) map[string]bool {
	if len(paths) == 0 {
		return nil
	}
	cmd := exec.Command("git", "check-ignore", "--stdin", "-z")
	cmd.Dir = root
	cmd.Stdin = strings.NewReader(strings.Join(paths, "\x00") + "\x00")
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &bytes.Buffer{}
	if err := cmd.Run(); err != nil {
		// check-ignore exits 1 when no path is ignored — an answer, not a failure.
		if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
			return map[string]bool{}
		}
		return nil
	}
	out := buf.String()
	ignored := map[string]bool{}
	for _, p := range strings.Split(out, "\x00") {
		if p != "" {
			ignored[p] = true
		}
	}
	return ignored
}

// AutoCommit stages the given paths and commits them — and ONLY them — with
// the standard evidence message (ADR D3). It runs mid-session (`shale done`)
// and from the pre-push hook, when the user may have their own work staged:
// `--only` with the pathspec keeps that work staged and out of the evidence
// commit. Reports whether a commit was actually created.
func AutoCommit(root string, paths []string, message string) (committed bool, err error) {
	if len(paths) == 0 {
		return false, nil
	}
	args := append([]string{"add", "--"}, paths...)
	if _, err := run(root, args...); err != nil {
		return false, err
	}
	staged, _ := run(root, append([]string{"diff", "--cached", "--name-only", "--"}, paths...)...)
	if strings.TrimSpace(staged) == "" {
		return false, nil
	}
	commit := append([]string{"commit", "--no-verify", "--only", "-m", message, "--"}, paths...)
	if _, err := run(root, commit...); err != nil {
		return false, err
	}
	return true, nil
}
