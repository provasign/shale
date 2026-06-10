// Package redact removes secrets from text before it is persisted anywhere
// outside .shale/local/. The ruleset is a vendored subset of the gitleaks
// default config covering the highest-frequency credential shapes. Only the
// hit COUNT is ever reported — never the matched content (plan A3).
package redact

import (
	"fmt"
	"regexp"
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
	}}
}

// Apply replaces every match with a [REDACTED:<rule>] placeholder and
// returns the cleaned text plus the number of hits. The matched content is
// intentionally not surfaced anywhere, including logs.
func (e *Engine) Apply(s string) (string, int) {
	total := 0
	for _, r := range e.rules {
		s = r.re.ReplaceAllStringFunc(s, func(string) string {
			total++
			return fmt.Sprintf("[REDACTED:%s]", r.name)
		})
	}
	return s, total
}
