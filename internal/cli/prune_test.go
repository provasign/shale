package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// prune deletes evidence finalized before the cutoff, keeps newer and
// unparseable files, and --dry-run deletes nothing.
func TestPruneByAge(t *testing.T) {
	root := chrepo(t)
	dir := filepath.Join(root, ".shale")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name string, finalized time.Time) {
		body := "shale_version: \"0\"\nid: " + name + "\ncreated_at: " + finalized.Add(-time.Hour).Format(time.RFC3339) +
			"\nfinalized_at: " + finalized.Format(time.RFC3339) + "\nprivacy: redacted\n"
		if err := os.WriteFile(filepath.Join(dir, name+".yaml"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now().UTC()
	write("old", now.Add(-60*24*time.Hour))
	write("new", now.Add(-2*24*time.Hour))
	os.WriteFile(filepath.Join(dir, "broken.yaml"), []byte(": : not yaml ["), 0o644)
	os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("privacy: redacted\n"), 0o644)

	code, out, errOut := run(t, "", "prune", "--older-than", "30d", "--dry-run")
	if code != 0 || !strings.Contains(out, "would delete .shale/old.yaml") || !strings.Contains(out, "kept 1 that do not parse") {
		t.Fatalf("dry-run: %d %q %q", code, out, errOut)
	}
	if _, err := os.Stat(filepath.Join(dir, "old.yaml")); err != nil {
		t.Fatal("dry-run deleted a file")
	}
	code, out, _ = run(t, "", "prune", "--older-than", "30")
	if code != 0 || !strings.Contains(out, "deleted .shale/old.yaml") {
		t.Fatalf("prune: %d %q", code, out)
	}
	for name, want := range map[string]bool{"old.yaml": false, "new.yaml": true, "broken.yaml": true, "config.yaml": true} {
		_, err := os.Stat(filepath.Join(dir, name))
		if (err == nil) != want {
			t.Errorf("%s exists=%v, want %v", name, err == nil, want)
		}
	}
	if code, _, _ := run(t, "", "prune"); code != 2 {
		t.Errorf("prune without --older-than: rc=%d, want 2", code)
	}
}
