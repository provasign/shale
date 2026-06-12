package gitx

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func initRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, args := range [][]string{
		{"init", "-b", "main"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "Test"},
		{"config", "commit.gpgsign", "false"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return root
}

func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, rel)
	os.MkdirAll(filepath.Dir(path), 0o755)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRootAndBranch(t *testing.T) {
	root := initRepo(t)
	sub := filepath.Join(root, "a", "b")
	os.MkdirAll(sub, 0o755)
	got := Root(sub)
	want, _ := filepath.EvalSymlinks(root)
	gotResolved, _ := filepath.EvalSymlinks(got)
	if gotResolved != want {
		t.Fatalf("Root = %q want %q", gotResolved, want)
	}
	writeFile(t, root, "f.txt", "x")
	if committed, err := AutoCommit(root, []string{"f.txt"}, "init"); err != nil || !committed {
		t.Fatalf("autocommit: committed=%v err=%v", committed, err)
	}
	if b := Branch(root); b != "main" {
		t.Fatalf("Branch = %q", b)
	}
	if r := Root(t.TempDir()); r != "" {
		t.Fatalf("Root outside repo = %q, want empty", r)
	}
}

func TestFilesChangedSince(t *testing.T) {
	root := initRepo(t)
	writeFile(t, root, "old.go", "package old")
	writeFile(t, root, "doomed.go", "package doomed")
	// Backdate the base commit so it falls outside the session window.
	past := time.Now().Add(-2 * time.Hour).Format(time.RFC3339)
	add := exec.Command("git", "add", "old.go", "doomed.go")
	add.Dir = root
	if out, err := add.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v\n%s", err, out)
	}
	commit := exec.Command("git", "commit", "--no-verify", "-m", "base")
	commit.Dir = root
	commit.Env = append(os.Environ(), "GIT_AUTHOR_DATE="+past, "GIT_COMMITTER_DATE="+past)
	if out, err := commit.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v\n%s", err, out)
	}

	since := time.Now().Add(-time.Minute)
	writeFile(t, root, "committed.go", "package a")
	if committed, err := AutoCommit(root, []string{"committed.go"}, "change"); err != nil || !committed {
		t.Fatalf("committed=%v err=%v", committed, err)
	}
	writeFile(t, root, "old.go", "package old\n// changed")
	writeFile(t, root, "uncommitted.go", "package b")
	writeFile(t, root, ".shale/x.yaml", "noise: true")
	rm := exec.Command("git", "rm", "-q", "doomed.go")
	rm.Dir = root
	if out, err := rm.CombinedOutput(); err != nil {
		t.Fatalf("git rm: %v\n%s", err, out)
	}

	got := FilesChangedSince(root, since)
	want := map[string][]string{
		"committed.go":   {"write"}, // added in a commit inside the window
		"old.go":         {"edit"},  // modified in the worktree
		"uncommitted.go": {"write"}, // untracked
		"doomed.go":      {"delete"},
	}
	if len(got) != len(want) {
		t.Fatalf("FilesChangedSince = %v", got)
	}
	for _, fc := range got {
		wantOps, ok := want[fc.Path]
		if !ok {
			t.Fatalf("unexpected path %q in %v", fc.Path, got)
		}
		if len(fc.Ops) != len(wantOps) || fc.Ops[0] != wantOps[0] {
			t.Errorf("%s ops = %v, want %v", fc.Path, fc.Ops, wantOps)
		}
	}
}

// AutoCommit runs mid-session now (`shale done`): the user's own staged work
// must stay staged and out of the evidence commit.
func TestAutoCommitLeavesUserStagingAlone(t *testing.T) {
	root := initRepo(t)
	writeFile(t, root, "base.txt", "x")
	if _, err := AutoCommit(root, []string{"base.txt"}, "base"); err != nil {
		t.Fatal(err)
	}
	writeFile(t, root, "wip.go", "package wip")
	stage := exec.Command("git", "add", "wip.go")
	stage.Dir = root
	if out, err := stage.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v\n%s", err, out)
	}
	writeFile(t, root, ".shale/s.yaml", "shale_version: \"0\"\n")

	committed, err := AutoCommit(root, []string{".shale/s.yaml"}, "evidence")
	if err != nil || !committed {
		t.Fatalf("committed=%v err=%v", committed, err)
	}
	show := exec.Command("git", "show", "--name-only", "--pretty=format:", "HEAD")
	show.Dir = root
	out, _ := show.Output()
	if strings.Contains(string(out), "wip.go") || !strings.Contains(string(out), ".shale/s.yaml") {
		t.Fatalf("evidence commit contents wrong:\n%s", out)
	}
	staged := exec.Command("git", "diff", "--cached", "--name-only")
	staged.Dir = root
	out, _ = staged.Output()
	if !strings.Contains(string(out), "wip.go") {
		t.Fatalf("user's staged work lost: %q", out)
	}
}

func TestAutoCommitNoopWhenNothingStaged(t *testing.T) {
	root := initRepo(t)
	writeFile(t, root, "f.txt", "x")
	if committed, err := AutoCommit(root, []string{"f.txt"}, "first"); err != nil || !committed {
		t.Fatalf("committed=%v err=%v", committed, err)
	}
	// Same paths again with no changes: must be a clean no-op.
	if committed, err := AutoCommit(root, []string{"f.txt"}, "second"); err != nil || committed {
		t.Fatalf("noop autocommit: committed=%v err=%v", committed, err)
	}
	if committed, err := AutoCommit(root, nil, "empty"); err != nil || committed {
		t.Fatalf("empty autocommit: committed=%v err=%v", committed, err)
	}
}
