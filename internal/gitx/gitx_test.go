package gitx

import (
	"os"
	"os/exec"
	"path/filepath"
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
	if err := AutoCommit(root, []string{"f.txt"}, "init"); err != nil {
		t.Fatalf("autocommit: %v", err)
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
	// Backdate the base commit so it falls outside the session window.
	past := time.Now().Add(-2 * time.Hour).Format(time.RFC3339)
	add := exec.Command("git", "add", "old.go")
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
	if err := AutoCommit(root, []string{"committed.go"}, "change"); err != nil {
		t.Fatal(err)
	}
	writeFile(t, root, "uncommitted.go", "package b")
	writeFile(t, root, ".shale/x.yaml", "noise: true")

	got := FilesChangedSince(root, since)
	want := map[string]bool{"committed.go": true, "uncommitted.go": true}
	if len(got) != len(want) {
		t.Fatalf("FilesChangedSince = %v", got)
	}
	for _, p := range got {
		if !want[p] {
			t.Fatalf("unexpected path %q in %v", p, got)
		}
	}
}

func TestAutoCommitNoopWhenNothingStaged(t *testing.T) {
	root := initRepo(t)
	writeFile(t, root, "f.txt", "x")
	if err := AutoCommit(root, []string{"f.txt"}, "first"); err != nil {
		t.Fatal(err)
	}
	// Same paths again with no changes: must be a clean no-op.
	if err := AutoCommit(root, []string{"f.txt"}, "second"); err != nil {
		t.Fatalf("noop autocommit errored: %v", err)
	}
	if err := AutoCommit(root, nil, "empty"); err != nil {
		t.Fatalf("empty autocommit errored: %v", err)
	}
}
