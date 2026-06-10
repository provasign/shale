# Shale — Decision records

Settled questions. The implementing agent should not relitigate these; if one
proves wrong in practice, surface the evidence to the maintainer instead of
silently diverging.

## D1 — License: Apache-2.0 (not AGPL)

The CLI runs on every developer laptop; many enterprise OSS policies blanket-ban
AGPL, and AGPL's network trigger protects nothing here (the value is the
workflow + spec, not a hostable server). Adoption *is* the moat — the spec only
wins if other tools can emit it freely. Commercial capture is future hosted-tier work
(server-side signing, policy, retention), not in this repo's license.

## D2 — Language: Go everywhere; composite (not TypeScript) Action

Single static binary for the laptop (no Node/Python runtime on dev machines),
cross-compilation, family consistency with provasign/grove/prism/fuse, and MVP 3
needs to embed `grove/pkg/grove` in-process. The GitHub Action is ~50 lines of
download-and-exec glue; a TypeScript action would add a second toolchain and a
`dist/` bundling pipeline for no capability gain.

## D3 — Evidence transport: committed files in `.shale/` (not git notes, not a server) for v1

Considered: (a) committed files, (b) git notes, (c) upload to a server.

- (c) violates the zero-server promise — rejected for v1 (it's a future hosted tier).
- (b) git notes don't alter SHAs (nice) but: not pushed/fetched by default,
  invisible in every git UI, bind to SHAs so rebase/squash orphans them, and
  fork contributors need extra ref permissions. This was a real source of
  complexity in the the predecessor system's ADR-006 design.
- (a) committed files survive rebase/squash *with* the code, are visible and
  diffable in the PR itself (the evidence is reviewable!), work from forks with
  zero setup, and work on any git host. Cost: a `chore(shale)` commit on push
  and evidence living in the repo. Accepted for v1; notes mode returns as an
  MVP 3 opt-in for teams that reject evidence commits.

### D3a — Repo growth and editability (the costs of D3, managed not denied)

- **Size:** shale YAMLs are 2–5 KB; even 50 PRs/week × 2 sessions ≈ ~500 KB/yr
  compressed — negligible. The genuine weight is transcripts, so committed
  transcripts are **prompts-only by default** (`transcript.kind: prompts` —
  user prompts + timestamps + one-line turn summaries, never tool outputs);
  `full` is explicit opt-in. `shale gc` (MVP 3) prunes the working tree for
  long-merged PRs; git history remains the audit record.
- **Editability:** see D11.

## D4 — Capture mechanism: steering prompt → CLI (no MCP); hooks are an enhancement tier

**Intent and completion are agent-declared, not hook-inferred.** Hooks fire
deterministically on file edits and commands — but hooks cannot know *why*
the agent is making changes or whether the work is semantically complete.
Inferring intent from the first user prompt produces garbage (exploratory
questions, "continue", "why is this failing?"). an earlier internal system we built proved this:
the agent declares intent before editing and closes it after finishing — that
timing and that authorship (agent's understood interpretation, not raw user
text) is what makes the intent meaningful.

**Evidence is captured in three tiers:**

1. **Semantic tier (universal — every agent, MVP 1).** `shale init` writes a
   steering prompt into every detected agent instruction file (CLAUDE.md,
   AGENTS.md, .cursorrules, .windsurfrules, .clinerules,
   `.github/copilot-instructions.md`, GEMINI.md, …). The agent calls
   `shale intent "<title>" [--body "..."]` before the first file edit and
   `shale done [...]` when the task is complete. These are plain CLI calls —
   any agent that can run a shell command (i.e., all of them, including
   Copilot agent mode) is covered with zero per-agent code.
2. **Mechanical tier (per-agent, best-effort).** Where the agent has a hook
   system, `shale capture <adapter>` records file touches, commands + exit
   codes, and session metadata deterministically. Claude Code in MVP 1;
   Cursor/Codex/Gemini in MVP 2. Hooks *upgrade card quality* (hook-verified
   file evidence, recorded checks) — they are never required for the core value.
3. **Fallback tier.** `shale finalize` fills `files[]` from `git diff` over the
   session window (intent → done) when no hook events exist, marked
   `via: git` so the card can distinguish hook-verified from git-derived
   evidence.

**Why CLI and not MCP tools? (settled against production data, 2026-06)**

- **Token pinning.** MCP tool calls and results pin to the agent's context
  window for the whole session; CLI output flows through the normal
  compression path and is evictable. an earlier internal system we built measured MCP integration at
  33% of session tokens in real sessions and reversed course to CLI-first
  (the predecessor system's ADR-006, "Agent integration — CLI over MCP"). Shale's payloads
  are small either way, but the asymmetry is structural and choosing right
  costs nothing.
- **Registration burden.** MCP needs a per-agent server entry — `.mcp.json`,
  `.cursor/mcp.json`, `.vscode/mcp.json`, `~/.codex/config.toml`, Windsurf,
  Zed, Continue, Kiro — across JSON and TOML formats with known idempotency
  traps (the predecessor system carries ~400 lines of config-merge code plus a bug history
  for exactly this). The CLI needs only PATH.
- **Approval friction.** Claude Code asks the user to approve new MCP servers
  — a human step injected into the 5-minute flow. A brew-installed binary has
  no ceremony.
- **Coverage.** Every agent that can edit files can run a shell command; not
  every agent speaks MCP. Steering + CLI is the only truly universal channel.

**Why not pure hooks for intent?** Hook events fire on every action; they have
no way to identify which prompt is the "real" intent vs. an exploratory
question. Only the agent, following a steering instruction, can declare intent
at the right moment with the right framing.

Fallbacks for agents with no steering-file support at all: `shale wrap`
(PTY wrapper) + `shale note` (manual annotation).

## D5 — Posture: advisory and fail-open everywhere in v1

Capture errors never break the agent's hook chain; finalize errors never block
a push; the Check Run is never `failure`. Trust products die when they add
friction before they've proven value. The first enforcement knob (strict mode)
arrives in MVP 3, opt-in, scoped to missing-shale/out-of-scope only.

## D6 — No telemetry, no accounts, no network calls from the laptop in v1

An evidence/provenance tool gets exactly one chance with the security persona.
The laptop half is verifiably offline (this is testable and tested). Growth is
measured through the Action's nudge-comment loop and marketplace installs, not
phone-home.

## D7 — Shale does not run gates

Scanner vendors (Sonar, Semgrep, Snyk) ship MCP servers; agents already
self-correct in-loop; CI is authoritative. Shale *records* agent-invoked
commands and their exit codes (`commands[]`) and renders them. The moment this
project grows a "run semgrep" feature it has re-entered the commodity fight the
parent project just exited. Hard no.

## D8 — Granularity: session/file level (not line-level attribution)

Git AI already does line-level AI attribution well and it's a different
question ("how much code is AI") than ours ("did the agent do the right
thing"). Session→file granularity is sufficient for the reviewer card and an
order of magnitude simpler. Revisit only with user pull.

## D9 — Naming

**Shale** (locked). The geological metaphor — layers that record time and
history — aligns with Grove (forest/structure), Prism (optical clarity), and
Fuse (joining/merging). All identifiers are mechanical: `shale` CLI, `.shale/`
dir, `shale-action`. If renamed later, it's a global find-replace plus
binary/tap/marketplace renames — keep identifiers mechanical, no clever wordplay.

## D10 — CI-agnostic core; the GitHub Action is packaging

Most enterprises run Jenkins or GitLab CI, not GitHub Actions. Therefore the
renderer is a plain binary with an env contract (`SHALE_FORGE`,
`SHALE_TOKEN`, `SHALE_REPO`, `SHALE_PR` — architecture §3.7) and a forge
driver interface (`github` MVP 1, `gitlab` MVP 2, `bitbucket` backlog). The
Action exists for OSS reach and marketplace distribution only. Any feature that
works *only* inside Actions is rejected.

## D11 — Threat model: tamper-evident in v1, tamper-proof is a future hosted tier

Committed shale can be hand-edited later; we say so openly instead of
pretending otherwise (full table: architecture §5.0). v1 detections — transcript
hash verification and "shale modified after its introducing commit" — surface
as ⚠️ card flags, never silently. Wholesale fabrication by a malicious author is
explicitly out of scope for v1: defeating it requires an independent witness
(Sigstore/Rekor co-signing, server-side verification), which is precisely the
future hosted enterprise tier. This boundary is both the honest engineering line
and the commercial upgrade reason — do not ship half a signing scheme in this
repo to blur it.

## D12 — Fork PRs: `pull_request_target` + zero checkout

Fork PRs get a read-only `GITHUB_TOKEN` under plain `pull_request`, which makes
posting the card impossible — and forks are exactly where evidence matters
most. We use `pull_request_target` (write-capable, base-repo context) and make
it safe **structurally**: the job never checks out or executes PR code; diff
and shale contents are fetched via the API as data; all shale content is
sanitized before entering the privileged comment (it is attacker-controlled
input). Adding a head-checkout step to this workflow is the one forbidden
change — `shale doctor` warns if it sees one.
