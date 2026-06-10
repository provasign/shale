package prcard

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/provasign/shale/internal/render"
	"github.com/provasign/shale/internal/store"
	"gopkg.in/yaml.v3"
)

// fakeForge is an in-memory Forge for orchestration tests.
type fakeForge struct {
	head     string
	files    []render.ChangedFile
	contents map[string][]byte   // path → bytes at head
	commits  []string            // PR commits
	touching map[string][]string // path → commits touching it
	comments []string
	statuses []string
}

func (f *fakeForge) PRHead(context.Context, int) (string, error) { return f.head, nil }
func (f *fakeForge) PRFiles(context.Context, int) ([]render.ChangedFile, error) {
	return f.files, nil
}
func (f *fakeForge) ListDir(_ context.Context, _, dir string) ([]string, error) {
	var out []string
	for p := range f.contents {
		if strings.HasPrefix(p, dir+"/") && !strings.Contains(strings.TrimPrefix(p, dir+"/"), "/") {
			out = append(out, p)
		}
	}
	return out, nil
}
func (f *fakeForge) FileContent(_ context.Context, _, path string) ([]byte, error) {
	if b, ok := f.contents[path]; ok {
		return b, nil
	}
	return nil, fmt.Errorf("404 not found: %s", path)
}
func (f *fakeForge) PRCommits(context.Context, int) ([]string, error) { return f.commits, nil }
func (f *fakeForge) CommitsTouching(_ context.Context, _, path string) ([]string, error) {
	return f.touching[path], nil
}
func (f *fakeForge) UpsertComment(_ context.Context, _ int, _, body string) error {
	f.comments = append(f.comments, body)
	return nil
}
func (f *fakeForge) SetStatus(_ context.Context, _, conclusion, title, _ string) error {
	f.statuses = append(f.statuses, conclusion+": "+title)
	return nil
}

func shaleBytes(t *testing.T, s *store.Shale) []byte {
	t.Helper()
	raw, err := yaml.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func validShale(id string, transcript []byte) *store.Shale {
	s := &store.Shale{
		ShaleVersion: "0",
		ID:           id,
		CreatedAt:    time.Date(2026, 6, 9, 14, 2, 11, 0, time.UTC),
		FinalizedAt:  time.Date(2026, 6, 9, 14, 41, 3, 0, time.UTC),
		Agent:        store.Agent{Tool: "claude-code", Model: "claude-fable-5"},
		Repo:         store.Repo{RootHint: "r"},
		Privacy:      store.PrivacyRedacted,
		Intent: &store.Intent{
			Title: "Fix the thing", TitleSHA256: "abc123",
			DeclaredAt: time.Date(2026, 6, 9, 14, 2, 11, 0, time.UTC),
		},
		Files: []store.FileTouch{{Path: "main.go", Ops: []string{"edit"}, Via: store.ViaHook}},
		Notes: []store.Note{},
	}
	if transcript != nil {
		s.Transcript = &store.Transcript{
			Path: "transcripts/" + id + ".md", SHA256: sha256hex(transcript), Kind: "prompts",
		}
	}
	return s
}

func TestRunHappyPath(t *testing.T) {
	transcript := []byte("# transcript")
	s := validShale("SESS01", transcript)
	f := &fakeForge{
		head:  "headsha",
		files: []render.ChangedFile{{Path: "main.go", Status: "modified"}},
		contents: map[string][]byte{
			".shale/SESS01.yaml":           shaleBytes(t, s),
			".shale/transcripts/SESS01.md": transcript,
			".shale/config.yaml":           []byte("privacy: redacted\n"),
		},
		commits:  []string{"c1", "c2"},
		touching: map[string][]string{".shale/SESS01.yaml": {"c2"}},
	}
	res, err := Run(context.Background(), f, 7)
	if err != nil {
		t.Fatal(err)
	}
	if res.Shales != 1 || res.Nudged || len(res.Tampered) != 0 {
		t.Fatalf("res = %+v", res)
	}
	if len(f.comments) != 1 || !strings.Contains(f.comments[0], "Fix the thing") {
		t.Fatalf("comments = %v", f.comments)
	}
	if strings.Contains(f.comments[0], "invalid shale file") {
		t.Fatalf("config.yaml must not be parsed as a session file: %s", f.comments[0])
	}
	if len(f.statuses) != 1 || !strings.HasPrefix(f.statuses[0], "neutral:") {
		t.Fatalf("statuses = %v", f.statuses)
	}
}

func TestRunNudgeWhenNoShale(t *testing.T) {
	f := &fakeForge{
		head:     "headsha",
		files:    []render.ChangedFile{{Path: "main.go", Status: "modified"}},
		contents: map[string][]byte{},
	}
	res, err := Run(context.Background(), f, 7)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Nudged || res.Shales != 0 {
		t.Fatalf("res = %+v", res)
	}
	if !strings.Contains(f.comments[0], "No shale for this PR") {
		t.Fatalf("nudge body wrong: %s", f.comments[0])
	}
}

func TestRunFlagsTranscriptTamper(t *testing.T) {
	s := validShale("SESS01", []byte("original"))
	f := &fakeForge{
		head:  "headsha",
		files: []render.ChangedFile{{Path: "main.go", Status: "modified"}},
		contents: map[string][]byte{
			".shale/SESS01.yaml":           shaleBytes(t, s),
			".shale/transcripts/SESS01.md": []byte("EDITED AFTER THE FACT"),
		},
	}
	res, err := Run(context.Background(), f, 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Tampered) != 1 || !strings.Contains(res.Tampered[0], "hash mismatch") {
		t.Fatalf("tamper = %v", res.Tampered)
	}
	if !strings.Contains(f.comments[0], "⚠️") {
		t.Fatal("tamper warning missing from card")
	}
}

func TestRunFlagsShaleEditedAfterCapture(t *testing.T) {
	s := validShale("SESS01", nil)
	f := &fakeForge{
		head:  "headsha",
		files: []render.ChangedFile{{Path: "main.go", Status: "modified"}},
		contents: map[string][]byte{
			".shale/SESS01.yaml": shaleBytes(t, s),
		},
		commits:  []string{"c1", "c2", "c3"},
		touching: map[string][]string{".shale/SESS01.yaml": {"c1", "c3"}}, // introduced in c1, edited in c3
	}
	res, err := Run(context.Background(), f, 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Tampered) != 1 || !strings.Contains(res.Tampered[0], "edited after capture") {
		t.Fatalf("tamper = %v", res.Tampered)
	}
}

func TestRunHostileShaleDegradesToWarning(t *testing.T) {
	f := &fakeForge{
		head:  "headsha",
		files: []render.ChangedFile{{Path: "main.go", Status: "modified"}},
		contents: map[string][]byte{
			".shale/evil.yaml": []byte("shale_version: \"0\"\nid: x\nprivacy: bogus-mode\n"),
			".shale/junk.yaml": []byte("{{{{not yaml"),
		},
	}
	res, err := Run(context.Background(), f, 7)
	if err != nil {
		t.Fatal(err)
	}
	if res.Shales != 0 {
		t.Fatalf("invalid shales were accepted: %+v", res)
	}
	if len(res.Tampered) != 2 {
		t.Fatalf("want 2 warnings, got %v", res.Tampered)
	}
}
