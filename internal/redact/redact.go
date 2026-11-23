// Package redact removes secrets from text before it is persisted anywhere
// outside .shale/local/. The ruleset is a vendored subset of the gitleaks
// default config covering the highest-frequency credential shapes. Only the
// hit COUNT is ever reported — never the matched content (plan A3).
package redact

import (
	"fmt"
	"regexp"
	"strings"
)

type rule struct {
	name string
	re   *regexp.Regexp
}

// Engine applies the redaction ruleset.
type Engine struct {
	rules []rule
}

// New returns an Engine with the default vendored ruleset.
func New() *Engine {
	mk := func(name, pattern string) rule {
		return rule{name: name, re: regexp.MustCompile(pattern)}
	}
	return &Engine{rules: []rule{
		mk("private-key", `-----BEGIN[ A-Z]*PRIVATE KEY-----(?s:.*?)(?:-----END[ A-Z]*PRIVATE KEY-----|\z)`),
		mk("aws-access-key", `\b(?:AKIA|ASIA|ABIA|ACCA)[0-9A-Z]{16}\b`),
		mk("github-token", `\b(?:ghp|gho|ghu|ghs|ghr)_[0-9A-Za-z]{36,255}\b`),
		mk("github-pat", `\bgithub_pat_[0-9A-Za-z_]{82}\b`),
		mk("gitlab-token", `\bglpat-[0-9A-Za-z\-=_]{20,22}\b`),
		mk("anthropic-key", `\bsk-ant-[0-9A-Za-z\-_]{20,}\b`),
		mk("openai-key", `\bsk-(?:proj-)?[0-9A-Za-z\-_]{20,}\b`),
		mk("slack-token", `\bxox[baprs]-[0-9A-Za-z\-]{10,}\b`),
		mk("stripe-key", `\b[rs]k_live_[0-9A-Za-z]{20,}\b`),
		mk("google-api-key", `\bAIza[0-9A-Za-z\-_]{35}\b`),
		mk("npm-token", `\bnpm_[0-9A-Za-z]{36}\b`),
		mk("jwt", `\beyJ[0-9A-Za-z_\-]{10,}\.[0-9A-Za-z_\-]{10,}\.[0-9A-Za-z_\-]{5,}\b`),
		mk("generic-assignment", `(?i)\b(?:api[_\-]?key|secret|passwd|password|token|auth)\b\s*[:=]\s*['"][^'"\s]{8,}['"]`),
		mk("url-credentials", `\b[a-zA-Z][a-zA-Z0-9+.-]*://[^/\s:@]+:[^@\s]+@`),
		// Command-line shapes: agent-run commands carry secrets as env
		// prefixes (API_KEY=x ./run), inline flags (--token=x), spaced flags
		// (--password x), and auth headers (Bearer x). Over-redacting a
		// harmless value costs a little card fidelity; leaking costs trust.
		mk("env-secret", `(?i)\b[A-Z0-9_]*(?:KEY|TOKEN|SECRET|PASSWORD|PASSWD|CREDENTIALS?)=[^\s'"]{6,}`),
		mk("flag-secret", `(?i)(?:^|\s)--[a-z0-9-]*(?:key|token|secret|password|passwd)(?:=|\s+)(?:"[^"]+"|'[^']+'|[^\s'"]+)`),
		mk("bearer-token", `(?i)\bbearer\s+[0-9A-Za-z._~+/=\-]{16,}`),
	}}
}

// homePath matches a home directory, macOS (/Users/<name>), Linux
// (/home/<name>) or Windows (C:\Users\<name>), up to the separator after
// the user name. Evidence is committed, often to a public repository, and a
// home path names the person and the machine (241 of 324 files in one
// repository carried /Users/<name>).
var homePath = regexp.MustCompile(`(?:/Users|/home|[A-Za-z]:\\Users)[/\\][^/\\\s"'\x60:;,)]+`)

// redactHome rewrites home directories to ~. It is not a secret, so it is
// not counted as a hit; it keeps the rest of the path readable.
func redactHome(s string) string {
	return homePath.ReplaceAllStringFunc(s, func(m string) string {
		// "/Users/Shared" and "/home" with no user are not personal.
		if strings.HasSuffix(m, "/Shared") || strings.HasSuffix(m, `\Public`) {
			return m
		}
		return "~"
	})
}

// Apply replaces every match with a [REDACTED:<rule>] placeholder and
// returns the cleaned text plus the number of hits. The matched content is
// intentionally not surfaced anywhere, including logs. Home directories
// become ~ (not counted).
func (e *Engine) Apply(s string) (string, int) {
	s = redactHome(s)
	total := 0
	for _, r := range e.rules {
		s = r.re.ReplaceAllStringFunc(s, func(string) string {
			total++
			return fmt.Sprintf("[REDACTED:%s]", r.name)
		})
	}
	return s, total
}

// Clean is Apply without the count.
func (e *Engine) Clean(s string) string {
	out, _ := e.Apply(s)
	return out
}
