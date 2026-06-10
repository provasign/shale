package render

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/provasign/shale/internal/store"
)

var update = flag.Bool("update", false, "rewrite golden files")

var (
	declared  = time.Date(2026, 6, 9, 14, 2, 11, 0, time.UTC)
	finalized = time.Date(2026, 6, 9, 14, 41, 3, 0, time.UTC)
)

func intPtr(i int) *int { return &i }

func sampleShale(id string) *store.Shale {
	touched := declared.Add(3 * time.Minute)
	return &store.Shale{
		ShaleVersion: "0",
		ID:           id,
		CreatedAt:    declared,
		FinalizedAt:  finalized,
		Agent:        store.Agent{Tool: "claude-code", Model: "claude-fable-5"},
		Repo:         store.Repo{RootHint: "my-repo", Branch: "feat/rate-limit"},
		Privacy:      store.PrivacyRedacted,
		Intent: &store.Intent{
			Title:       "Add rate limiting to the login endpoint",
			Body:        "Brute force attempts observed in prod logs. Redis counter, 10 req/min per IP.",
			TitleSHA256: "9f1c2ab4d3e5f6a7",
			DeclaredAt:  declared,
			PromptCount: 14,
		},
		Completion: &store.Completion{
			Note:  "Redis-backed rate limiter implemented. In-memory fallback added.",
			Model: "claude-fable-5", TokensIn: 32000, TokensOut: 15000,
			TokensTotal: 47000, CostUSD: 0.47, Iterations: 3,
		},
		Transcript: &store.Transcript{Path: "transcripts/" + id + ".md", SHA256: "4be91d03aabbccdd", Kind: "prompts"},
		Files: []store.FileTouch{
			{Path: "internal/auth/ratelimit.go", Ops: []string{"write"}, Via: store.ViaHook, FirstTouched: &touched},
			{Path: "internal/auth/ratelimit_test.go", Ops: []string{"write"}, Via: store.ViaHook},
			{Path: "internal/auth/login.go", Ops: []string{"edit"}, Via: store.ViaHook},
		},
		Commands: []store.Command{
			{Cmd: "gitleaks detect --no-banner", ExitCode: intPtr(0), At: declared.Add(29 * time.Minute), Classified: "scan"},
			{Cmd: "go test ./internal/auth/...", ExitCode: intPtr(0), At: declared.Add(31 * time.Minute), Classified: "test"},
		},
	}
}

func samplePRFiles() []ChangedFile {
	return []ChangedFile{
		{Path: "internal/auth/ratelimit.go", Status: "added"},
		{Path: "internal/auth/ratelimit_test.go", Status: "added"},
		{Path: "internal/auth/login.go", Status: "modified"},
		{Path: "go.mod", Status: "modified"},
		{Path: ".github/workflows/deploy.yml", Status: "modified"},
	}
}

func checkGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", "golden", name)
	if *update {
		os.MkdirAll(filepath.Dir(path), 0o755)
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("golden missing (run with -update): %v", err)
	}
	if got != string(want) {
		t.Errorf("golden mismatch for %s:\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
	}
}

func TestCardSingleSession(t *testing.T) {
	got := Card(Input{Shales: []*store.Shale{sampleShale("a1b2c3d4e5")}, PRFiles: samplePRFiles()})
	checkGolden(t, "single_session.md", got)
	for _, must := range []string{
		"🧾 Shale · 1 session",
		"47k tokens", "~$0.47", "3 iterations", "38 min",
		"**Add rate limiting to the login endpoint**",
		"✅ a1b2c3d4e5",
		"dependency manifest", "CI config",
		"Checks recorded locally", "✅ passed",
		"with evidence",
		CommentMarker,
	} {
		if !strings.Contains(got, must) {
			t.Errorf("card missing %q", must)
		}
	}
}

func TestCardIgnoresShaleEvidenceFiles(t *testing.T) {
	// The evidence files finalize commits must not show up as coverage gaps
	// — otherwise every clean instrumented PR is flagged.
	s := sampleShale("a1b2c3d4e5")
	s.Files = []store.FileTouch{{Path: "main.go", Ops: []string{"edit"}, Via: store.ViaHook}}
	got := Card(Input{
		Shales: []*store.Shale{s},
		PRFiles: []ChangedFile{
			{Path: "main.go", Status: "modified"},
			{Path: ".shale/sess01.yaml", Status: "added"},
			{Path: ".shale/transcripts/sess01.md", Status: "added"},
		},
	})
	if strings.Contains(got, ".shale/") {
		t.Errorf("evidence files must not appear in the file table:\n%s", got)
	}
	if !strings.Contains(got, "all with session evidence") {
		t.Errorf("changed-files count must exclude evidence files:\n%s", got)
	}
}

func TestCardMultiSession(t *testing.T) {
	s2 := sampleShale("f6g7h8i9j0")
	s2.Agent.Model = "claude-sonnet-4-6"
	s2.Intent.Title = "Add tests for the fallback path"
	s2.Intent.Body = ""
	got := Card(Input{
		Shales:  []*store.Shale{sampleShale("a1b2c3d4e5"), s2},
		PRFiles: samplePRFiles(),
	})
	checkGolden(t, "multi_session.md", got)
	if !strings.Contains(got, "2 sessions") {
		t.Error("multi-session count missing")
	}
}

func TestCardNoShale(t *testing.T) {
	got := Card(Input{PRFiles: samplePRFiles()})
	checkGolden(t, "no_shale.md", got)
	for _, must := range []string{NudgeMarker, "No shale for this PR", "humans don't"} {
		if !strings.Contains(got, must) {
			t.Errorf("nudge missing %q", must)
		}
	}
}

func TestCardGitDerivedEvidence(t *testing.T) {
	s := sampleShale("k1l2m3n4o5")
	s.Files = []store.FileTouch{
		{Path: "internal/auth/login.go", Ops: []string{"edit"}, Via: store.ViaGit},
	}
	got := Card(Input{Shales: []*store.Shale{s}, PRFiles: samplePRFiles()})
	checkGolden(t, "git_derived.md", got)
	if !strings.Contains(got, "◐ k1l2m3n4o5") || !strings.Contains(got, "not hook-verified") {
		t.Error("git-derived evidence must be visually distinct (spec rule 9)")
	}
}

func TestCardNoIntentDeclared(t *testing.T) {
	s := sampleShale("p1q2r3s4t5")
	s.Intent = nil
	got := Card(Input{Shales: []*store.Shale{s}, PRFiles: samplePRFiles()})
	if !strings.Contains(got, "No intent declared") {
		t.Error("absence of intent must be explicit, never silent")
	}
}

func TestCardTamperFlags(t *testing.T) {
	got := Card(Input{
		Shales:  []*store.Shale{sampleShale("a1b2c3d4e5")},
		PRFiles: samplePRFiles(),
		TamperFlags: []string{
			"transcript hash mismatch for session a1b2c3 — treat intent text as unverified",
			"shale a1b2c3 edited after capture",
		},
	})
	checkGolden(t, "tamper_flags.md", got)
	if strings.Count(got, "> ⚠️") != 2 {
		t.Error("tamper flags must render prominently")
	}
}

func TestCardLargePR(t *testing.T) {
	var files []ChangedFile
	for i := 0; i < 200; i++ {
		files = append(files, ChangedFile{Path: fmt.Sprintf("pkg/gen/file_%03d.go", i), Status: "modified"})
	}
	files = append(files, ChangedFile{Path: ".github/workflows/deploy.yml", Status: "modified"})
	got := Card(Input{Shales: []*store.Shale{sampleShale("a1b2c3d4e5")}, PRFiles: files})
	checkGolden(t, "large_pr.md", got)
	if !strings.Contains(got, "<details>") {
		t.Error("large PR must collapse the full list")
	}
	if !strings.Contains(got, "| `pkg/` | 200 |") {
		t.Errorf("directory grouping missing")
	}
	// Sensitive path stays above the fold even in a 200-file PR.
	fold := strings.Index(got, "<details>")
	if !strings.Contains(got[:fold], "deploy.yml") {
		t.Error("sensitive path fell below the fold")
	}
}

func TestCardHostileShaleCannotInject(t *testing.T) {
	s := sampleShale("evil123456")
	s.Intent.Title = `<img src=x onerror=alert(1)> @everyone fix #1 [click](https://evil.example)`
	s.Intent.Body = "<script>steal()</script> ping @maintainer"
	s.Completion.Note = "done & dusted <iframe src=evil>"
	s.Files[0].Path = "internal/<b>bold</b>.go"
	s.Commands[0].Cmd = "echo `@all` #42"
	got := Card(Input{Shales: []*store.Shale{s}, PRFiles: samplePRFiles()})
	checkGolden(t, "hostile.md", got)
	for _, banned := range []string{
		"<img", "<script>", "<iframe", "<b>", "](https://evil.example)",
	} {
		if strings.Contains(got, banned) {
			t.Errorf("hostile content survived sanitization: %q", banned)
		}
	}
	// Raw mention/ref tokens must be broken (zero-width space inserted).
	for _, banned := range []string{"@everyone", "@maintainer", "@all", "#42", "#1"} {
		if strings.Contains(got, banned) {
			t.Errorf("mention/ref not neutralized: %q", banned)
		}
	}
}

func TestSanitize(t *testing.T) {
	cases := map[string]string{
		"plain text":     "plain text",
		"<b>x</b>":       "&lt;b&gt;x&lt;/b&gt;",
		"a & b":          "a &amp; b",
		"hi @user":       "hi @\u200buser",
		"fix #12":        "fix #\u200b12",
		"[t](http://x)":  "[t]\u200b(http://x)",
		"ctrl\x01char":   "ctrlchar",
		"keep\nnewlines": "keep\nnewlines",
	}
	for in, want := range cases {
		if got := Sanitize(in); got != want {
			t.Errorf("Sanitize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSensitiveReason(t *testing.T) {
	cases := map[string]string{
		"go.mod":                     "dependency manifest",
		"web/package.json":           "dependency manifest",
		".github/workflows/x.yml":    "CI config",
		".gitlab-ci.yml":             "CI config",
		"deploy/Jenkinsfile":         "CI/build config",
		"infra/main.tf":              "infrastructure as code",
		"internal/auth/login.go":     "auth/crypto path",
		"pkg/crypto/aes.go":          "auth/crypto path",
		"README.md":                  "",
		"internal/author_profile.go": "", // "author" is not "auth"
	}
	for p, want := range cases {
		if got := sensitiveReason(p); got != want {
			t.Errorf("sensitiveReason(%q) = %q, want %q", p, got, want)
		}
	}
}
