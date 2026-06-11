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

// FilesChangedSince returns repo-relative paths changed in the window from t
// to now: commits authored since t on the current branch plus any
// uncommitted (staged or unstaged) changes. This feeds the `via: git`
// fallback (ADR D4 tier 3) — honest about being a window, not a touch log.
func FilesChangedSince(root string, t time.Time) []string {
	seen := map[string]bool{}

	logOut, err := run(root,
		"log", "--since="+t.UTC().Format(time.RFC3339),
		"--name-only", "--pretty=format:", "HEAD")
	if err == nil {
		for _, line := range strings.Split(logOut, "\n") {
			if line = strings.TrimSpace(line); line != "" {
				seen[line] = true
			}
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
			path = strings.Trim(path, `"`)
			if path != "" {
				seen[path] = true
			}
		}
	}

	paths := make([]string, 0, len(seen))
	for p := range seen {
		// Evidence about the evidence directory is noise.
		if p == ".shale" || strings.HasPrefix(p, ".shale/") {
			continue
		}
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return paths
}

// HooksPath returns the repo's core.hooksPath override resolved to an
// absolute path, or "" when unset/unavailable so callers fall back to
// .git/hooks. Hook managers (husky, lefthook) set this — a hook written to
// .git/hooks in those repos would never run.
func HooksPath(root string) string {
	out, err := run(root, "config", "--type=path", "--get", "core.hooksPath")
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

// AutoCommit stages the given paths and commits them with the standard
// evidence message (ADR D3). Returns nil when there is nothing to commit.
func AutoCommit(root string, paths []string, message string) error {
	if len(paths) == 0 {
		return nil
	}
	args := append([]string{"add", "--"}, paths...)
	if _, err := run(root, args...); err != nil {
		return err
	}
	staged, _ := run(root, "diff", "--cached", "--name-only")
	if strings.TrimSpace(staged) == "" {
		return nil
	}
	_, err := run(root, "commit", "--no-verify", "-m", message)
	return err
}
