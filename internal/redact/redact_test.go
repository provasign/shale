package redact

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyRedactsSeededSecrets(t *testing.T) {
	e := New()
	cases := []struct {
		name string
		in   string
	}{
		{"aws", "creds AKIAIOSFODNN7EXAMPLE in prompt"},
		{"github ghp", "use ghp_aBcDeFgHiJkLmNoPqRsTuVwXyZ0123456789 to push"},
		{"github pat", "github_pat_" + strings.Repeat("a", 82)},
		{"gitlab", "glpat-aBcDeFgHiJkLmNoPqRsT"},
		{"anthropic", "key sk-ant-api03-aBcDeFgHiJkLmNoPqRsTuVwX"},
		{"openai", "sk-proj-aBcDeFgHiJkLmNoPqRsTuVwXyZ123456"},
		{"slack", "xoxb-1234567890-abcdefghij"},
		{"stripe", "sk_live_aBcDeFgHiJkLmNoPqRsT"},
		{"google", "AIza" + strings.Repeat("a", 35)},
		{"npm", "npm_" + strings.Repeat("a", 36)},
		{"jwt", "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U"},
		{"private key", "-----BEGIN RSA PRIVATE KEY-----\nMIIEpAIBAAKCAQEA\n-----END RSA PRIVATE KEY-----"},
		{"assignment", `password = "hunter2hunter2"`},
		{"url creds", "postgres://admin:s3cr3tpass@db.internal:5432/app"},
		{"env prefix", "API_KEY=zXcVbNm123 ./deploy.sh --env prod"},
		{"export env", "export GITHUB_TOKEN=zXcVbNm123456"},
		{"inline flag", "curl --token=zXcVbNm123 https://api.internal/v1"},
		{"spaced flag", "mytool --password hunter2hunter2 --verbose"},
		{"build arg", "docker build --build-arg NPM_TOKEN=zXcVbNm123 ."},
		{"bearer header", `curl -H "Authorization: Bearer zXcVbNm1234567890abc"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, n := e.Apply(tc.in)
			if n == 0 {
				t.Fatalf("secret not detected in %q", tc.in)
			}
			if !strings.Contains(out, "[REDACTED:") {
				t.Fatalf("no placeholder in output %q", out)
			}
			// The secret material itself must be gone.
			for _, frag := range []string{"AKIAIOSFODNN7", "hunter2", "s3cr3tpass", "MIIEpAIBAAKCAQEA", "zXcVbNm1"} {
				if strings.Contains(tc.in, frag) && strings.Contains(out, frag) {
					t.Fatalf("secret material survived redaction: %q", out)
				}
			}
		})
	}
}

func TestApplyLeavesCleanTextAlone(t *testing.T) {
	e := New()
	in := "Add rate limiting to the login endpoint. Redis counter, 10 req/min per IP."
	out, n := e.Apply(in)
	if n != 0 || out != in {
		t.Fatalf("clean text mutated: n=%d out=%q", n, out)
	}
}

func TestApplyLeavesCleanCommandsAlone(t *testing.T) {
	e := New()
	for _, in := range []string{
		"go test ./internal/auth/...",
		"git commit -m \"fix token parsing logic\"",
		"npm run build -- --mode production",
		"kubectl get secrets -n default",
		"golangci-lint run --timeout 5m",
		"docker build -t app:latest .",
	} {
		if out, n := e.Apply(in); n != 0 || out != in {
			t.Errorf("clean command mutated: n=%d in=%q out=%q", n, in, out)
		}
	}
}

func TestApplyCountsMultipleHits(t *testing.T) {
	e := New()
	in := "a AKIAIOSFODNN7EXAMPLE b AKIAIOSFODNN7EXAMPLE c xoxb-1234567890-abcdefghij"
	_, n := e.Apply(in)
	if n != 3 {
		t.Fatalf("want 3 hits, got %d", n)
	}
}

func TestTermsMatchWholeWordsIgnoringCase(t *testing.T) {
	e := New().WithTerms([]string{"zeph", "Big Co", "c++lib", "a.b"})
	cases := map[string]string{
		"run zeph now":       "run [REDACTED:term] now",
		"ZEPH/specs":         "[REDACTED:term]/specs",
		"zephyr stays":       "zephyr stays",
		"big co ships":       "[REDACTED:term] ships",
		"use c++lib, ok":     "use [REDACTED:term], ok",
		"a.b and axb":        "[REDACTED:term] and axb",
		"pre_zeph is a word": "pre_zeph is a word",
	}
	for in, want := range cases {
		if got, _ := e.Apply(in); got != want {
			t.Errorf("Apply(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLoadTermsSkipsCommentsAndRefusesMissing(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "t.txt")
	os.WriteFile(p, []byte("# c\n\n Zeph \nzeph\nAcme\n"), 0o644)
	terms, err := LoadTerms(p)
	if err != nil || len(terms) != 2 || terms[0] != "Zeph" || terms[1] != "Acme" {
		t.Fatalf("terms %q, %v", terms, err)
	}
	if _, err := LoadTerms(filepath.Join(dir, "missing.txt")); err == nil {
		t.Fatal("a missing named list loaded")
	}
}
