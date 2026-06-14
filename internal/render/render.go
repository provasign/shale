// Package render builds the Shale card: a pure function from shale files +
// the PR file list to GitHub-flavored markdown (docs/product.md §3).
// All shale-derived text is attacker-controlled data and passes through
// Sanitize before entering the card (workstream D6).
package render

import (
	"fmt"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/provasign/shale/internal/commandfmt"
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
	writeHookValidationNotes(&b, in.Shales)
	writeIntents(&b, in.Shales, in.BlobBase)
	writeCompletions(&b, in.Shales)
	writeFiles(&b, in)
	writeChecks(&b, in.Shales)
	return b.String()
}

// logoIcon is the Shale strata mark, rendered inline in the card heading.
// The <picture> element serves a contrast-lifted variant on GitHub's dark
// themes (both PNGs are transparent — no white chip in dark mode). GitHub
// proxies the images through camo and caches them; if the host is ever
// unreachable the empty alt keeps the heading readable.
const logoIcon = `<picture><source media="(prefers-color-scheme: dark)" srcset="https://provasign.dev/assets/images/logo-card-dark.png"><img src="https://provasign.dev/assets/images/logo-card-light.png" width="20" height="20" alt=""></picture>`

// Nudge is the exact no-shale copy from docs/product.md §3.
func Nudge() string {
	return NudgeMarker + `
## ` + logoIcon + ` No shale for this PR
This repo renders agent evidence on PRs. No agent session evidence was found
for these commits. If you used an AI agent: ` + "`brew tap provasign/shale && brew install shale && shale init`" + `
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
		if model := modelForSession(s); model != "" {
			models[model] = true
		}
	}
	head := fmt.Sprintf("## %s Shale · %d session%s", logoIcon, len(shales), plural(len(shales)))
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
	var missingTokens, missingCost bool
	var dur time.Duration
	for _, s := range shales {
		if c := s.Completion; c != nil {
			tokens += c.TokensTotal
			iterations += c.Iterations
			if c.CostUSD > 0 {
				cost += c.CostUSD
				haveCost = true
			} else {
				missingCost = true
			}
			if c.TokensTotal == 0 {
				missingTokens = true
			}
		} else {
			missingTokens = true
			missingCost = true
		}
		if s.Intent != nil && !s.FinalizedAt.IsZero() {
			dur += s.FinalizedAt.Sub(s.Intent.DeclaredAt)
		}
	}
	if len(models) == 1 {
		parts = append(parts, Sanitize(sortedKeys(models)[0]))
	}
	if tokens > 0 {
		label := formatTokens(tokens) + " tokens"
		if missingTokens {
			label = formatTokens(tokens) + " known tokens"
		}
		parts = append(parts, label)
	}
	if haveCost {
		label := fmt.Sprintf("~$%.2f", cost)
		if missingCost {
			label += " known cost"
		}
		parts = append(parts, label)
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

func writeHookValidationNotes(b *strings.Builder, shales []*store.Shale) {
	var unverified []string
	for _, s := range shales {
		if len(s.Files) == 0 {
			continue
		}
		haveHook := false
		for _, f := range s.Files {
			if f.Via == store.ViaHook {
				haveHook = true
				break
			}
		}
		if !haveHook {
			unverified = append(unverified, displayID(s.ID))
		}
	}
	if len(unverified) == 0 {
		return
	}
	noun, verb := "Sessions", "have"
	if len(unverified) == 1 {
		noun, verb = "Session", "has"
	}
	fmt.Fprintf(b, "\n> ℹ️ %s `%s` only %s git fallback file evidence: Shale saw files change while the session was active, but no agent hook reported those edits. Token and command totals may be incomplete.\n",
		noun, Sanitize(strings.Join(unverified, "`, `")), verb)
}

func writeIntents(b *strings.Builder, shales []*store.Shale, blobBase string) {
	b.WriteString("\n### Intent\n")
	wrote := false
	for _, s := range shales {
		if s.Intent == nil || s.Intent.Title == "" {
			continue
		}
		if wrote {
			// Blank line between stacked intents so they don't run together.
			b.WriteString("\n")
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
			formatTimeUTC(s.Intent.DeclaredAt), Sanitize(displayID(s.ID)))
		for _, part := range sessionMetaParts(s, true) {
			meta += " · " + part
		}
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

func modelForSession(s *store.Shale) string {
	if s == nil {
		return ""
	}
	if s.Agent.Model != "" {
		return s.Agent.Model
	}
	if s.Completion != nil {
		return s.Completion.Model
	}
	return ""
}

func sessionMetaParts(s *store.Shale, includeUnknowns bool) []string {
	var parts []string
	if s.Agent.Tool != "" {
		parts = append(parts, "agent `"+Sanitize(s.Agent.Tool)+"`")
	}
	if model := modelForSession(s); model != "" {
		parts = append(parts, "model `"+Sanitize(model)+"`")
	} else if includeUnknowns {
		parts = append(parts, "model unknown")
	}
	if s.Completion != nil && s.Completion.TokensTotal > 0 {
		parts = append(parts, formatTokens(s.Completion.TokensTotal)+" tokens")
	} else if includeUnknowns {
		parts = append(parts, "tokens unknown")
	}
	if s.Completion != nil && s.Completion.CostUSD > 0 {
		parts = append(parts, fmt.Sprintf("~$%.2f", s.Completion.CostUSD))
	} else if includeUnknowns {
		parts = append(parts, "cost unknown")
	}
	if s.Completion != nil && s.Completion.Iterations > 0 {
		parts = append(parts, fmt.Sprintf("%d iteration%s", s.Completion.Iterations, plural(s.Completion.Iterations)))
	}
	if s.Intent != nil && !s.FinalizedAt.IsZero() {
		if d := s.FinalizedAt.Sub(s.Intent.DeclaredAt); d > 0 {
			parts = append(parts, formatDuration(d))
		}
	}
	return parts
}

func writeCompletions(b *strings.Builder, shales []*store.Shale) {
	type note struct {
		sessionID string
		text      string
	}
	var notes []note
	for _, s := range shales {
		if s.Completion != nil && s.Completion.Note != "" {
			notes = append(notes, note{sessionID: displayID(s.ID), text: s.Completion.Note})
		}
	}
	if len(notes) == 0 {
		return
	}
	b.WriteString("\n### Completion\n")
	for _, n := range notes {
		fmt.Fprintf(b, "> **%s** · %s\n", code(Sanitize(n.sessionID)), Sanitize(n.text))
	}
}

// fileEvidence is what the sessions claim about one path.
type fileEvidence struct {
	sessionID string
	via       string
}

type fileRow struct {
	path, sessionID, evidence, notes string
	flagged                          bool
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

	var rows []fileRow
	seen := 0
	for _, cf := range in.PRFiles {
		r := fileRow{path: cf.Path}
		if ev, ok := evidence[cf.Path]; ok {
			seen++
			r.sessionID = ev.sessionID
			switch ev.via {
			case store.ViaHook:
				r.evidence = "✅ hook event"
			default:
				r.evidence = "◐ git fallback"
				r.notes = "changed while session was active; no agent hook event recorded"
			}
		} else {
			r.sessionID = "—"
			r.evidence = "—"
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
		fmt.Fprintf(b, "\n### Changed files (%d) — %d with evidence · %d without session evidence\n",
			len(in.PRFiles), seen, untracked)
	} else {
		fmt.Fprintf(b, "\n### Changed files (%d) — all with session evidence\n", len(in.PRFiles))
	}
	b.WriteString("\n*Legend: ✅ hook event = an agent hook reported the file edit; ◐ git fallback = the file changed while that session was active, but no hook event was recorded; — = no session evidence matched the PR file.*\n")

	sort.SliceStable(rows, func(i, j int) bool {
		return rowLess(rows[i], rows[j])
	})

	writeRow := func(r fileRow) {
		fmt.Fprintf(b, "| %s | %s | %s | %s |\n",
			code(Sanitize(r.sessionID)), r.evidence, code(Sanitize(r.path)), r.notes)
	}
	header := "| Session ID | Evidence | File | Notes |\n|---|---|---|---|\n"

	if len(rows) <= maxFileRows {
		b.WriteString(header)
		for _, r := range rows {
			writeRow(r)
		}
		return
	}

	// Large PR: flagged rows above the fold, the rest grouped by directory
	// inside a collapsible block (docs/product.md §3).
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
		sessionID, cmd, result, when string
		at                           time.Time
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
				sessionID: displayID(s.ID), cmd: commandfmt.ForDisplay(c.Cmd), result: result,
				when: formatTimeUTC(c.At), at: c.At,
			})
		}
	}
	if len(checks) == 0 {
		return
	}
	// Group by session, then chronological — same session-first ordering as the
	// file table, so a multi-session PR shows which session ran each check
	// instead of an undifferentiated duplicate list.
	sort.SliceStable(checks, func(i, j int) bool {
		if checks[i].sessionID != checks[j].sessionID {
			return checks[i].sessionID < checks[j].sessionID
		}
		return checks[i].at.Before(checks[j].at)
	})
	b.WriteString("\n### Checks recorded locally\n| Session ID | Check | Result | When |\n|---|---|---|---|\n")
	for _, c := range checks {
		fmt.Fprintf(b, "| %s | %s | %s | %s |\n", code(Sanitize(c.sessionID)), code(Sanitize(c.cmd)), c.result, c.when)
	}
	b.WriteString("\n*Advisory — CI is authoritative.*\n")
}

// sensitiveReason flags paths a security reviewer must see above the fold.
// Built-in list per docs/product.md §5; policy.yaml override is MVP 3.
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

func rowLess(a, b fileRow) bool {
	aUnmatched := a.sessionID == "—"
	bUnmatched := b.sessionID == "—"
	if aUnmatched != bUnmatched {
		return !aUnmatched
	}
	if a.sessionID != b.sessionID {
		return a.sessionID < b.sessionID
	}
	if a.evidence != b.evidence {
		return a.evidence < b.evidence
	}
	return a.path < b.path
}

func formatTimeUTC(t time.Time) string {
	if t.IsZero() {
		return "unknown time"
	}
	return t.UTC().Format("2006-01-02 15:04 UTC")
}

func formatDuration(d time.Duration) string {
	switch {
	case d >= time.Minute:
		return fmt.Sprintf("%d min", int(d.Minutes()))
	case d > 0:
		return "< 1 min"
	default:
		return ""
	}
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

func code(s string) string {
	s = strings.ReplaceAll(s, "`", "'")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "|", `\|`)
	return "`" + s + "`"
}

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
