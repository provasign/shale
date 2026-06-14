package commandfmt

import (
	"strings"
	"testing"
)

func TestForEvidenceExtractsCheckAndDropsHeredoc(t *testing.T) {
	raw := `cat > pkg/query_test.go <<'EOF'
package pkg

const token = "ghp_aBcDeFgHiJkLmNoPqRsTuVwXyZ0123456789"
EOF
cd /Users/noahkreiger/Documents/fianulabs/core/core; go test ./pkg/criteria -run TestTranslator_NumericPropertyComparison -v 2>&1 | grep -E "PASS|FAIL" | head -25`

	got := ForEvidence("/Users/noahkreiger/Documents/fianulabs/core/core", raw)
	if got != "go test ./pkg/criteria -run TestTranslator_NumericPropertyComparison -v 2>&1" {
		t.Fatalf("ForEvidence() = %q", got)
	}
	for _, banned := range []string{"package pkg", "ghp_", "/Users/noahkreiger", "grep -E", "head -25"} {
		if strings.Contains(got, banned) {
			t.Fatalf("ForEvidence leaked %q in %q", banned, got)
		}
	}
}

func TestForDisplayRedactsHistoricalLocalPaths(t *testing.T) {
	raw := "cd /Users/noahkreiger/Documents/fianulabs/core/core; go test ./pkg/... 2>&1 | tail -20"
	got := ForDisplay(raw)
	if got != "go test ./pkg/... 2>&1" {
		t.Fatalf("ForDisplay() = %q", got)
	}
	if strings.Contains(got, "/Users/noahkreiger") {
		t.Fatalf("ForDisplay leaked local path: %q", got)
	}
}

func TestClassify(t *testing.T) {
	cases := map[string]string{
		"go test ./...":     "test",
		"gitleaks detect":   "scan",
		"golangci-lint run": "lint",
		"go build ./...":    "build",
		"ls -la":            "other",
	}
	for in, want := range cases {
		if got := Classify(in); got != want {
			t.Errorf("Classify(%q) = %q, want %q", in, got, want)
		}
	}
}
