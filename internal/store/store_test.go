package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func intPtr(i int) *int { return &i }

// specExample mirrors the worked example in docs/shale-spec.md §2.
func specExample(t *testing.T) *Shale {
	t.Helper()
	created := time.Date(2026, 6, 9, 14, 2, 11, 0, time.UTC)
	finalized := time.Date(2026, 6, 9, 14, 41, 3, 0, time.UTC)
	touched := time.Date(2026, 6, 9, 14, 5, 2, 0, time.UTC)
	return &Shale{
		ShaleVersion: "0",
		ID:           "01J9ZK7Q4N8WPXG2",
		CreatedAt:    created,
		FinalizedAt:  finalized,
		Agent: Agent{
			Tool: "claude-code", ToolVersion: "2.1.0",
			Model: "claude-fable-5", AdapterVersion: "0.3.0",
		},
		Repo:    Repo{RootHint: "my-repo", Branch: "feat/login-rate-limit"},
		Privacy: PrivacyRedacted,
		Intent: &Intent{
			Title:       "Add rate limiting to the login endpoint",
			Body:        "Brute force attempts observed in prod logs. Redis counter, 10 req/min\nper IP, return 429 with Retry-After header. Tests required.\n",
			TitleSHA256: "9f1c2ab4",
			BodySHA256:  "3d7e91f2",
			DeclaredAt:  created,
			PromptCount: 14,
		},
		Completion: &Completion{
			Note:        "Redis-backed rate limiter implemented.",
			Model:       "claude-fable-5",
			TokensIn:    32000,
			TokensOut:   15000,
			TokensTotal: 47000,
			CostUSD:     0.47,
			Iterations:  3,
		},
		Transcript: &Transcript{
			Path: "transcripts/01J9ZK7Q4N8WPXG2.md", SHA256: "4be91d03", Kind: "prompts",
		},
		Files: []FileTouch{
			{Path: "internal/auth/ratelimit.go", Ops: []string{"write"}, Via: ViaHook, FirstTouched: &touched},
			{Path: "internal/auth/login.go", Ops: []string{"edit"}, Via: ViaHook},
		},
		Commands: []Command{
			{Cmd: "go test ./internal/auth/...", ExitCode: intPtr(0), At: finalized, Classified: "test"},
			{Cmd: "gitleaks detect --no-banner", ExitCode: intPtr(0), At: finalized, Classified: "scan"},
		},
		Notes:      []Note{},
		Redactions: 2,
	}
}

func TestRoundTripSpecExample(t *testing.T) {
	s := specExample(t)
	path := filepath.Join(t.TempDir(), "s.yaml")
	if err := s.Save(path); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.ID != s.ID || got.Intent.Title != s.Intent.Title ||
		got.Completion.CostUSD != s.Completion.CostUSD ||
		len(got.Files) != 2 || got.Files[0].Via != ViaHook ||
		*got.Commands[0].ExitCode != 0 || got.Redactions != 2 {
		t.Fatalf("round trip mismatch: %+v", got)
	}
	if !got.Files[0].FirstTouched.Equal(*s.Files[0].FirstTouched) {
		t.Fatalf("first_touched mismatch")
	}
}

func TestAppendOnlyGuard(t *testing.T) {
	s := specExample(t)
	path := filepath.Join(t.TempDir(), "s.yaml")
	if err := s.Save(path); err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := s.Save(path); err == nil || !strings.Contains(err.Error(), "append-only") {
		t.Fatalf("expected append-only error, got %v", err)
	}
}

func TestParseRejectsUnknownMajorVersion(t *testing.T) {
	_, err := Parse([]byte("shale_version: \"9\"\nid: x\nprivacy: redacted\n"))
	if err == nil || !strings.Contains(err.Error(), "unsupported shale_version") {
		t.Fatalf("expected version error, got %v", err)
	}
}

func TestParseAcceptsUnknownFields(t *testing.T) {
	raw := "shale_version: \"0\"\nid: x\nprivacy: redacted\nfuture_field: hello\n"
	if _, err := Parse([]byte(raw)); err != nil {
		t.Fatalf("unknown fields must be accepted (spec rule 1): %v", err)
	}
}

func TestValidateRejections(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*Shale)
		want string
	}{
		{"bad privacy", func(s *Shale) { s.Privacy = "loud" }, "privacy"},
		{"missing title hash", func(s *Shale) { s.Intent.TitleSHA256 = "" }, "title_sha256"},
		{"bad via", func(s *Shale) { s.Files[0].Via = "psychic" }, "via"},
		{"absolute path", func(s *Shale) { s.Files[0].Path = "/etc/passwd" }, "absolute"},
		{"missing id", func(s *Shale) { s.ID = "" }, "id"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := specExample(t)
			tc.mut(s)
			err := s.Validate()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want error containing %q, got %v", tc.want, err)
			}
		})
	}
}

func TestEventsAppendReadArchive(t *testing.T) {
	root := t.TempDir()
	id := NewULID(time.Now())
	if len(id) != 26 {
		t.Fatalf("ulid length = %d", len(id))
	}
	events := []Event{
		{Kind: KindIntent, At: time.Now().UTC(), Title: "t", Body: "b"},
		{Kind: KindFileTouch, At: time.Now().UTC(), Path: "a.go", Op: "edit"},
		{Kind: KindCompletion, At: time.Now().UTC(), Note: "done", TokensIn: 5},
	}
	for _, ev := range events {
		if err := AppendEvent(root, id, ev); err != nil {
			t.Fatalf("append: %v", err)
		}
	}
	if err := SetCurrentSession(root, id); err != nil {
		t.Fatalf("set current: %v", err)
	}
	if got := CurrentSession(root); got != id {
		t.Fatalf("current = %q want %q", got, id)
	}
	got, err := ReadEvents(SessionPath(root, id))
	if err != nil || len(got) != 3 {
		t.Fatalf("read events: %v len=%d", err, len(got))
	}
	if got[0].Title != "t" || got[1].Path != "a.go" || got[2].TokensIn != 5 {
		t.Fatalf("event fields lost: %+v", got)
	}
	ids, err := ListSessions(root)
	if err != nil || len(ids) != 1 || ids[0] != id {
		t.Fatalf("list sessions: %v %v", ids, err)
	}
	if err := ArchiveSession(root, id); err != nil {
		t.Fatalf("archive: %v", err)
	}
	if got := CurrentSession(root); got != "" {
		t.Fatalf("current should be cleared, got %q", got)
	}
	ids, _ = ListSessions(root)
	if len(ids) != 0 {
		t.Fatalf("archived session still listed: %v", ids)
	}
}

func TestReadEventsSkipsMalformedLines(t *testing.T) {
	root := t.TempDir()
	path := SessionPath(root, "s1")
	os.MkdirAll(filepath.Dir(path), 0o755)
	content := `{"kind":"intent","title":"ok"}` + "\nnot json\n" + `{"kind":"note","text":"n"}` + "\n"
	os.WriteFile(path, []byte(content), 0o644)
	got, err := ReadEvents(path)
	if err != nil || len(got) != 2 {
		t.Fatalf("want 2 events skipping garbage, got %d (%v)", len(got), err)
	}
}

func TestULIDMonotonicPrefix(t *testing.T) {
	a := NewULID(time.UnixMilli(1))
	b := NewULID(time.UnixMilli(1 << 40))
	if a[:10] >= b[:10] {
		t.Fatalf("time prefix not ordered: %s vs %s", a, b)
	}
}
