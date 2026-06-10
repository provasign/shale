// Package render builds the Shale card: a pure function from shale files +
// the PR file list to GitHub-flavored markdown (docs/01-product.md §3.3).
// All shale-derived text is attacker-controlled data and passes through
// Sanitize before entering the card (workstream D6).
package render

import (
	"fmt"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/provasign/shale/internal/store"
)

// CommentMarker is the hidden marker used for comment upsert dedup.
const CommentMarker = "<!-- shale-card -->"

// NudgeMarker dedups the no-shale nudge (posted at most once per PR).
const NudgeMarker = "<!-- shale-nudge -->"

// maxFileRows is the changed-files table cap before directory grouping.
const maxFileRows = 20

// ChangedFile is one entry of the PR diff.
type ChangedFile struct {
	Path   string
	Status string // added | modified | removed | renamed
}

// Input is everything the card derives from. The renderer contract (spec
// rule 7): shale files + PR diff only, no network access to interpret.
type Input struct {
	Shales      []*store.Shale
	PRFiles     []ChangedFile
	TamperFlags []string // pre-computed warnings, one line each
	// BlobBase is the base URL for blob links, e.g.
	// "https://github.com/owner/repo/blob/<sha>". When set, transcript
	// references in the card become clickable links.
	BlobBase string
}

// Card renders the full card markdown, including the hidden upsert marker.
// With no shales it renders the explicit no-shale nudge (absence is never
// silent).
func Card(in Input) string {
	if len(in.Shales) == 0 {
		return Nudge()
	}
	// Shale's own evidence files ride along with every instrumented PR; they
	// are bookkeeping, not agent work product. Counting them as "changed with
	// no session evidence" would put a coverage gap on every clean PR.
	// Tamper detection (D7) still watches them upstream.
	kept := in.PRFiles[:0:0]
	for _, cf := range in.PRFiles {
		if cf.Path == ".shale" || strings.HasPrefix(cf.Path, ".shale/") {
			continue
		}
		kept = append(kept, cf)
	}
	in.PRFiles = kept
	var b strings.Builder
	b.WriteString(CommentMarker + "\n")
	writeHeader(&b, in.Shales)
	for _, w := range in.TamperFlags {
		fmt.Fprintf(&b, "\n> ⚠️ %s\n", Sanitize(w))
	}
	writeIntents(&b, in.Shales, in.BlobBase)
	writeCompletions(&b, in.Shales)
	writeFiles(&b, in)
	writeChecks(&b, in.Shales)
	return b.String()
}

// Nudge is the exact no-shale copy from docs/01-product.md §3.4.
func Nudge() string {
	return NudgeMarker + `
## 🧾 No shale for this PR
This repo renders agent evidence on PRs. No agent session evidence was found
for these commits. If you used an AI agent: install Shale from
https://github.com/provasign/shale/releases/latest and run ` + "`shale init`" + `
(5 minutes, no account). If this was hand-written, ignore this — humans don't
need shale. 🙂
`
}

func writeHeader(b *strings.Builder, shales []*store.Shale) {
	tools := map[string]bool{}
	models := map[string]bool{}
	for _, s := range shales {
		if s.Agent.Tool != "" {
			tools[s.Agent.Tool] = true
		}
		if s.Agent.Model != "" {
			models[s.Agent.Model] = true
		}
	}
	head := fmt.Sprintf("## 🧾 Shale · %d session%s", len(shales), plural(len(shales)))
	if len(tools) > 0 {
		head += " · " + Sanitize(strings.Join(sortedKeys(tools), ", "))
	}
	if len(models) > 0 {
		head += fmt.Sprintf(" (%s)", Sanitize(strings.Join(sortedKeys(models), ", ")))
	}
	b.WriteString(head + "\n")

	// Cost line: model · tokens · cost · iterations · duration.
	var parts []string
	var tokens, iterations int
	var cost float64
	var haveCost bool
	var dur time.Duration
	for _, s := range shales {
		if c := s.Completion; c != nil {
			tokens += c.TokensTotal
			iterations += c.Iterations
			if c.CostUSD > 0 {
				cost += c.CostUSD
				haveCost = true
			}
		}
		if s.Intent != nil && !s.FinalizedAt.IsZero() {
			dur += s.FinalizedAt.Sub(s.Intent.DeclaredAt)
		}
	}
	if len(models) == 1 {
		parts = append(parts, Sanitize(sortedKeys(models)[0]))
	}
	if tokens > 0 {
		parts = append(parts, formatTokens(tokens)+" tokens")
	}
	if haveCost {
		parts = append(parts, fmt.Sprintf("~$%.2f", cost))
	}
	if iterations > 0 {
		parts = append(parts, fmt.Sprintf("%d iteration%s", iterations, plural(iterations)))
	}
	switch {
	case dur >= time.Minute:
		parts = append(parts, fmt.Sprintf("%d min", int(dur.Minutes())))
	case dur > 0:
		parts = append(parts, "< 1 min")
	}
	if len(parts) > 0 {
		b.WriteString(strings.Join(parts, " · ") + "\n")
	}
}

func writeIntents(b *strings.Builder, shales []*store.Shale, blobBase string) {
	b.WriteString("\n### Intent\n")
	wrote := false
	for _, s := range shales {
		if s.Intent == nil || s.Intent.Title == "" {
			continue
		}
		wrote = true
		fmt.Fprintf(b, "> **%s**\n", Sanitize(s.Intent.Title))
		if s.Intent.Body != "" {
			b.WriteString(">\n")
			for _, line := range strings.Split(strings.TrimRight(s.Intent.Body, "\n"), "\n") {
				fmt.Fprintf(b, "> %s\n", Sanitize(line))
			}
		}
		meta := fmt.Sprintf("*Declared %s · session `%s`",
			s.Intent.DeclaredAt.UTC().Format("2006-01-02 15:04"), Sanitize(displayID(s.ID)))
		if s.Transcript != nil {
			hash := fmt.Sprintf("sha256:%s…", Sanitize(shortHash(s.Transcript.SHA256)))
			if blobBase != "" {
				url := blobBase + "/.shale/" + s.Transcript.Path
				meta += fmt.Sprintf(" · [transcript](%s) `%s`", url, hash)
			} else {
				meta += fmt.Sprintf(" · transcript `%s`", hash)
			}
		}
		b.WriteString("\n" + meta + "*\n")
	}
	if !wrote {
		// Absence is explicit, never silent (spec rule 5).
		b.WriteString("*No intent declared for this PR's sessions.*\n")
	}
}

func writeCompletions(b *strings.Builder, shales []*store.Shale) {
	var notes []string
	for _, s := range shales {
		if s.Completion != nil && s.Completion.Note != "" {
			notes = append(notes, s.Completion.Note)
		}
	}
	if len(notes) == 0 {
		return
	}
	b.WriteString("\n### Completion\n")
	for _, n := range notes {
		fmt.Fprintf(b, "> %s\n", Sanitize(n))
	}
}

// fileEvidence is what the sessions claim about one path.
type fileEvidence struct {
	sessionID string
	via       string
}

func writeFiles(b *strings.Builder, in Input) {
	evidence := map[string]fileEvidence{}
	for _, s := range in.Shales {
		for _, f := range s.Files {
			// Hook evidence beats git-derived evidence for the same path.
			if prev, ok := evidence[f.Path]; ok && prev.via == store.ViaHook {
				continue
			}
			evidence[f.Path] = fileEvidence{sessionID: displayID(s.ID), via: f.Via}
		}
	}

	type row struct {
		path, badge, notes string
		flagged            bool
	}
	var rows []row
	seen := 0
	for _, cf := range in.PRFiles {
		r := row{path: cf.Path}
		if ev, ok := evidence[cf.Path]; ok {
			seen++
			switch ev.via {
			case store.ViaHook:
				r.badge = "✅ " + ev.sessionID
			default:
				r.badge = "◐ " + ev.sessionID
				r.notes = "changed during session — not hook-verified"
			}
		} else {
			r.badge = "—"
			r.flagged = true
		}
		if reason := sensitiveReason(cf.Path); reason != "" {
			if r.notes != "" {
				r.notes += " · "
			}
			r.notes += "**sensitive path: " + reason + "**"
			r.flagged = true
		}
		if cf.Status == "added" && r.notes == "" {
			r.notes = "new file"
		}
		rows = append(rows, r)
	}

	untracked := len(in.PRFiles) - seen
	if untracked > 0 {
		fmt.Fprintf(b, "\n### Changed files (%d) — %d with evidence · %d untracked\n",
			len(in.PRFiles), seen, untracked)
	} else {
		fmt.Fprintf(b, "\n### Changed files (%d) — all with session evidence\n", len(in.PRFiles))
	}

	writeRow := func(r row) {
		fmt.Fprintf(b, "| %s | %s | %s |\n", code(Sanitize(r.path)), r.badge, r.notes)
	}
	header := "| File | Session ID | Notes |\n|---|---|---|\n"

	if len(rows) <= maxFileRows {
		b.WriteString(header)
		for _, r := range rows {
			writeRow(r)
		}
		return
	}

	// Large PR: flagged rows above the fold, the rest grouped by directory
	// inside a collapsible block (docs/01-product.md §3.3).
	b.WriteString(header)
	for _, r := range rows {
		if r.flagged {
			writeRow(r)
		}
	}
	groups := map[string]int{}
	for _, r := range rows {
		dir := topDir(r.path)
		groups[dir]++
	}
	b.WriteString("\nFiles grouped by directory:\n\n| Directory | Files |\n|---|---|\n")
	for _, dir := range sortedKeys(toBoolMap(groups)) {
		fmt.Fprintf(b, "| %s | %d |\n", code(Sanitize(dir)), groups[dir])
	}
	b.WriteString("\n<details><summary>Full file list</summary>\n\n")
	b.WriteString(header)
	for _, r := range rows {
		writeRow(r)
	}
	b.WriteString("\n</details>\n")
}

func writeChecks(b *strings.Builder, shales []*store.Shale) {
	type check struct {
		cmd, result, when string
	}
	var checks []check
	for _, s := range shales {
		for _, c := range s.Commands {
			if c.Classified == "other" || c.Classified == "build" {
				continue
			}
			result := "❔ exit unknown"
			if c.ExitCode != nil {
				if *c.ExitCode == 0 {
					result = "✅ passed"
				} else {
					result = fmt.Sprintf("❌ exit %d", *c.ExitCode)
				}
			}
			checks = append(checks, check{
				cmd: c.Cmd, result: result, when: c.At.UTC().Format("15:04"),
			})
		}
	}
	if len(checks) == 0 {
		return
	}
	b.WriteString("\n### Checks recorded locally\n| Check | Result | When |\n|---|---|---|\n")
	for _, c := range checks {
		fmt.Fprintf(b, "| %s | %s | %s |\n", code(Sanitize(c.cmd)), c.result, c.when)
	}
	b.WriteString("\n*Advisory — CI is authoritative.*\n")
}

// sensitiveReason flags paths a security reviewer must see above the fold.
// Built-in list per architecture §3.4; policy.yaml override is MVP 3.
func sensitiveReason(p string) string {
	base := path.Base(p)
	lower := strings.ToLower(p)
	switch base {
	case "go.mod", "go.sum", "package.json", "package-lock.json", "yarn.lock",
		"pnpm-lock.yaml", "requirements.txt", "Pipfile", "Pipfile.lock",
		"Gemfile", "Gemfile.lock", "pom.xml", "build.gradle", "Cargo.toml", "Cargo.lock":
		return "dependency manifest"
	case "Dockerfile", "Jenkinsfile":
		return "CI/build config"
	}
	switch {
	case strings.HasPrefix(p, ".github/workflows/"), base == ".gitlab-ci.yml",
		strings.HasPrefix(p, ".circleci/"):
		return "CI config"
	case strings.HasSuffix(base, ".tf"), strings.HasSuffix(base, ".tfvars"):
		return "infrastructure as code"
	case containsSegment(lower, "auth"), containsSegment(lower, "crypto"),
		containsSegment(lower, "secrets"):
		return "auth/crypto path"
	}
	return ""
}

// containsSegment reports whether any path segment contains sub — matching
// "internal/auth/x.go" but not "author_test.go" at the top level alone.
func containsSegment(p, sub string) bool {
	for _, seg := range strings.Split(p, "/") {
		if seg == sub || strings.HasPrefix(seg, sub+"_") || strings.HasPrefix(seg, sub+".") {
			return true
		}
	}
	return false
}

func topDir(p string) string {
	if i := strings.Index(p, "/"); i > 0 {
		return p[:i] + "/"
	}
	return "./"
}

// displayID shortens a session ID for the card. UUIDs (xxxxxxxx-xxxx-…) are
// trimmed to their first 8 hex chars. Human-readable IDs (no hyphens at
// position 8) are kept as-is — they're already short by convention.
func displayID(id string) string {
	if len(id) >= 36 && id[8] == '-' && id[13] == '-' {
		return id[:8]
	}
	return id
}

func shortHash(h string) string {
	if len(h) > 8 {
		return h[:8]
	}
	return h
}

func formatTokens(n int) string {
	if n >= 1000 {
		return fmt.Sprintf("%dk", (n+500)/1000)
	}
	return fmt.Sprintf("%d", n)
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func code(s string) string { return "`" + strings.ReplaceAll(s, "`", "'") + "`" }

func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func toBoolMap(m map[string]int) map[string]bool {
	out := make(map[string]bool, len(m))
	for k := range m {
		out[k] = true
	}
	return out
}
