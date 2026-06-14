// Package commandfmt turns raw agent shell invocations into review-safe
// command evidence. Hook payloads can contain whole shell scripts; committed
// shale should carry the check that ran, not heredoc file bodies or local
// machine paths.
package commandfmt

import (
	"path/filepath"
	"regexp"
	"strings"
)

const maxCommandLen = 220

var (
	homePathRe = regexp.MustCompile(`(?i)(?:/Users|/home)/[^/\s'"` + "`" + `;|)]+`)
	absPathRe  = regexp.MustCompile(`(^|[\s='":])/(?:[^\s'"` + "`" + `;|)]+)`)
	splitRe    = regexp.MustCompile(`\s*(?:&&|\|\||;|\n)\s*`)
	heredocRe  = regexp.MustCompile(`<<-?\s*['"]?([A-Za-z_][A-Za-z0-9_]*)['"]?`)
)

// ForEvidence returns the command form safe to persist in shale YAML.
func ForEvidence(repoRoot, raw string) string {
	return clean(repoRoot, raw)
}

// ForDisplay returns the command form safe to render for historical evidence
// that may predate persistence cleanup.
func ForDisplay(raw string) string {
	return clean("", raw)
}

// Classify buckets an agent-invoked command for the "checks recorded" card
// section. Heuristic by design; "other" is an honest answer.
func Classify(cmd string) string {
	c := strings.ToLower(cmd)
	switch {
	case containsAny(c, "gitleaks", "semgrep", "trivy", "snyk", "govulncheck", "bandit", "grype"):
		return "scan"
	case containsAny(c, "golangci-lint", "eslint", "ruff", "flake8", "pylint", "rubocop", "clippy", "go vet", "staticcheck", "tflint"):
		return "lint"
	case containsAny(c, "go test", "pytest", "npm test", "yarn test", "pnpm test", "jest", "vitest", "cargo test", "rspec", "mvn test", "gradle test", "make test"):
		return "test"
	case containsAny(c, "go build", "npm run build", "yarn build", "cargo build", "make build", "mvn package", "gradle build", "docker build", "tsc"):
		return "build"
	default:
		return "other"
	}
}

func clean(repoRoot, raw string) string {
	cmd := stripHeredocs(strings.ReplaceAll(raw, "\r\n", "\n"))
	for _, part := range commandParts(cmd) {
		part = cleanOne(repoRoot, part)
		if part != "" && Classify(part) != "other" {
			return part
		}
	}
	return cleanOne(repoRoot, cmd)
}

func stripHeredocs(s string) string {
	var out []string
	lines := strings.Split(s, "\n")
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		m := heredocRe.FindStringSubmatch(line)
		if len(m) == 0 {
			out = append(out, line)
			continue
		}
		end := m[1]
		for i+1 < len(lines) {
			i++
			if strings.TrimSpace(lines[i]) == end {
				break
			}
		}
	}
	return strings.Join(out, "\n")
}

func commandParts(s string) []string {
	var out []string
	for _, part := range splitRe.Split(s, -1) {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if i := strings.Index(part, "|"); i >= 0 {
			out = append(out, strings.TrimSpace(part[:i]))
			continue
		}
		out = append(out, part)
	}
	return out
}

func cleanOne(repoRoot, s string) string {
	s = filepath.ToSlash(s)
	if repoRoot != "" {
		root := filepath.ToSlash(filepath.Clean(repoRoot))
		s = strings.ReplaceAll(s, root, ".")
	}
	s = homePathRe.ReplaceAllString(s, "~")
	s = absPathRe.ReplaceAllString(s, "${1}<path>")
	s = strings.Join(strings.Fields(s), " ")
	if strings.HasPrefix(s, "cd .") {
		if i := strings.Index(s, ";"); i >= 0 {
			s = strings.TrimSpace(s[i+1:])
		}
	}
	if len(s) > maxCommandLen {
		s = strings.TrimSpace(s[:maxCommandLen-1]) + "…"
	}
	return s
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
