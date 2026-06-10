# Shale — Architecture

## 1. System overview

```
┌─ LAPTOP ────────────────────────────────────────────────────────────┐
│                                                                     │
│  agent (Claude Code / Cursor / Codex / Copilot / Gemini / any)      │
│     │                                                               │
│     │  [steering prompt written by `shale init` into CLAUDE.md /   │
│     │   AGENTS.md / .cursorrules / copilot-instructions.md / …]     │
│     │                                                               │
│     ├─ BEFORE first file edit:                                      │
│     │    agent calls `shale intent "<title>" [--body "..."]`        │
│     │    → written to .shale/local/<session>.jsonl                  │
│     │      (CLI, not MCP — universal + context-evictable, ADR D4)   │
│     │                                                               │
│     ├─ DURING session (where the agent has hooks — enhancement):    │
│     │    hooks fire on PostToolUse, Bash, Stop                      │
│     │    → `shale capture <adapter>` appends file-touch +           │
│     │       command + session-meta events (JSONL, gitignored)       │
│     │                                                               │
│     └─ AFTER work complete:                                         │
│          agent calls `shale done [--note "..."] [--tokens-in N      │
│                        --tokens-out N --model M --iterations N]`    │
│          → completion block written to .shale/local/<session>.jsonl │
│                                                                     │
│  git pre-push hook → `shale finalize`   (mechanical safety net)    │
│     │  sessions → redaction → .shale/<session>.yaml               │
│     │  no hook events? files[] filled from git diff (via: git)     │
│     │  auto-commits the shale files (ADR D3)                      │
│     ▼                                                               │
│  normal `git push`  (shale files ride along as committed files)     │
└─────────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
┌─ ANY CI (user's own — zero Shale servers) ────────────────────────┐
│  GitHub Actions / Jenkins / GitLab CI / CircleCI / …                │
│     runs the pinned `shale` binary (checksum-verified)              │
│     `shale render --pr <n>`                                       │
│        fetches PR diff + .shale/ contents via the forge API      │
│        (no code checkout required — see §3.7)                       │
│        builds card markdown (sanitized)                             │
│        upserts PR/MR comment + neutral status via forge driver      │
└─────────────────────────────────────────────────────────────────────┘
```

Two halves, one binary. There is no daemon, no database beyond flat files, no
network call from the laptop half at all.

**The Action is packaging, not the product.** `shale render` is a plain CLI
with a documented environment contract (§3.7); the composite GitHub Action is
one convenient wrapper around it. Enterprises on Jenkins/GitLab CI run the same
binary in their own pipeline — this is a first-class, documented path, not a
workaround (ADR D10).

## 2. Language and toolchain (settled — see ADR D2)

- **Go 1.26+** for the entire CLI and the Action's logic. One static binary,
  zero runtime deps, cross-compiles to mac/linux/windows · amd64/arm64, matches
  the same toolchain family (Grove, Prism), and lets MVP 3 embed `grove/pkg/grove`
  in-process. Module: `github.com/provasign/shale` (joins the root `go.work`).
- **The GitHub Action is a composite action** (`action/action.yml`): a few
  lines of shell that download the checksum-pinned binary for the runner's
  OS/arch and exec `shale render`. No Node toolchain, no `dist/` bundling, no
  second language. (TypeScript Action rejected: doubles the toolchain for ~50
  lines of glue.)
- SQLite, CGO, Redis, Postgres: **not used**. Storage is YAML/JSONL files.
- Lint/test conventions follow the sibling repos: `Makefile` with
  `build/test/lint/install`, table-driven tests, `golangci-lint`.

## 3. Components

### 3.1 `cmd/shale` — CLI

| Command | Role |
|---|---|
| `shale init` | Write steering prompt into every detected agent instruction file (CLAUDE.md, AGENTS.md, .cursorrules, .windsurfrules, .clinerules, `.github/copilot-instructions.md`, GEMINI.md, `.kiro/steering/` — this alone covers every agent), install hooks for agents that have them, scaffold `.shale/`, write workflow file, ask privacy mode. Idempotent; shows diffs before touching existing files. |
| `shale intent "<title>" [--body "..."]` | **Agent-called** (via steering prompt) BEFORE the first file edit. Declares what the agent is about to do and why. Never inferred from prompts. If absent, card shows "no intent declared". |
| `shale done [--note "..."] [--tokens-in N] [--tokens-out N] [--model M] [--iterations N]` | **Agent-called** (via steering prompt) AFTER work is complete. Semantic closure: the agent's self-report that the task is done. Token counts feed the cost line on the card. Analogous to `provasign intent close`. |
| `shale capture <adapter>` | Hook entry point (automatic). Reads the agent's hook JSON from stdin, normalizes to a capture event, appends file-touch + command + session-meta to `.shale/local/<session>.jsonl`. Must complete in <50 ms and **never fail the hook** (errors → log file, exit 0). |
| `shale finalize` | Mechanical safety net (pre-push hook). Folds all JSONL events → shale YAML; runs redaction; writes transcript + `sha256` hash; computes `cost_usd` from token counts + pricing table; **fills `files[]` from `git diff` over the session window when no hook events exist (`via: git`, ADR D4 tier 3)**; auto-commits per ADR D3. Safe to run manually. |
| `shale render` | CI-side. Build the card from PR diff + shale files; post/update comment and Check Run. Also `--local` to preview markdown in the terminal. |
| `shale note "<text>"` | Manual annotation escape hatch (not the happy path). |
| `shale verify <file>` | Validate a shale against the JSON Schema; recompute hashes. (MVP 2) |
| `shale doctor` | Diagnose: hooks installed? steering prompt present? events flowing? workflow present? last finalize? |

### 3.2 `internal/capture` — agent hook adapters (enhancement tier)

Hook adapters are **tier 2 of the capture model (ADR D4)**: they add
deterministic, hook-verified file/command evidence on top of the universal
semantic tier (`shale intent` / `shale done` via steering prompt). An agent
with no adapter still produces a full shale — intent, completion, cost, and a
git-derived file list — so adapter coverage gates card *quality*, never card
*existence*.

Adapter interface (keep it this small):

```go
type Adapter interface {
    Name() string                                  // "claude-code", "cursor", ...
    Detect(repoRoot string) (installed bool)       // is this agent configured here/globally?
    Install(repoRoot string, cfg InstallConfig) error  // wire hooks (idempotent, diff-and-ask)
    Parse(hookPayload []byte) ([]Event, error)     // hook stdin JSON → normalized events
}
```

Normalized `Event` kinds: `intent` (title + body from `shale intent` call),
`file_touch` (path + tool + edit/write/delete), `command` (command line +
exit code — feeds "checks recorded"), `session_meta` (agent, model, session
id, cwd), `session_end`, `completion` (note + token counts from `shale done`).

Note: `prompt` events (raw user text) are NOT captured as intent — capturing
raw prompts produces garbage. Intent is always a discrete `shale intent` call.

**Adapter notes (verify against current vendor docs at implementation time —
hook APIs move fast):**

- **claude-code** (MVP 1): hooks in `~/.claude/settings.json` or
  `.claude/settings.json` — `UserPromptSubmit` (prompt text),
  `PostToolUse` matcher on `Edit|Write|MultiEdit|NotebookEdit` (file touches)
  and on `Bash` (commands), `SessionStart`/`Stop` (session meta). Hook command:
  `shale capture claude-code`.
- **cursor** (MVP 2): Cursor hooks (`.cursor/hooks.json`, beta API).
- **codex** (MVP 2): Codex CLI `notify`/hook config; fall back to session-file
  tailing if hook coverage is insufficient.
- **gemini-cli / windsurf** (MVP 2+): same pattern.
- **copilot / anything without hooks** (MVP 1, by design): no adapter needed —
  the steering prompt in `.github/copilot-instructions.md` drives
  `shale intent`/`shale done`, and finalize's git fallback supplies `files[]`.
- **generic** (MVP 2): `shale wrap -- <agent-cmd>` PTY wrapper +
  `shale note` for agents with no hook system *and* no instruction file.

Adapters are the highest-maintenance surface of the product. Keep each one in
its own file with a fixture-based test (recorded real hook payloads in
`testdata/`).

### 3.2a Steering prompt — written by `shale init`

`shale init` appends a block to every detected agent instruction file
(CLAUDE.md, AGENTS.md, .cursorrules, .github/copilot-instructions.md,
GEMINI.md, etc.). The block is fenced with `<!-- shale-start -->` /
`<!-- shale-end -->` markers so it is idempotent and removable. Content:

```
<!-- shale-start -->
## Shale — agent evidence (do not remove)

Before editing any file, call:
  shale intent "<one line: what you are about to do>" [--body "<why, constraints, approach>"]

After completing the task (before asking for review), call:
  shale done [--note "<brief: what you did, any deviations>"] \
             [--tokens-in <n>] [--tokens-out <n>] \
             [--model <model-id>] [--iterations <n>]

Everything else (file tracking, command recording) is automatic.
<!-- shale-end -->
```

**Why this is not "ceremony":** the agent calls both commands automatically
by following these instructions — the user does nothing. Intent is the agent's
understood interpretation of the task, written at the moment of maximum
clarity (after reading the request, before touching files). This is the
predecessor system's lesson applied correctly: the agent writes the intent, not the user,
and it does so at precisely the right moment.

**Why CLI and not MCP tools (ADR D4):** MCP tool calls and their results pin
to the agent's context window for the whole session; CLI output goes through
the normal compression path and is evictable. an earlier internal system we built measured its MCP
integration at 33% of session tokens in production and moved its agent
workflow to CLI-first (the predecessor system's ADR-006, "Agent integration — CLI over
MCP"). Just as important: MCP needs a per-agent server registration across
8+ config formats plus a user approval step in Claude Code, while a CLI on
PATH needs nothing — and *every* agent can run a shell command, which is what
makes this tier universal (Copilot included, via
`.github/copilot-instructions.md`). Shale ships no MCP server. Both commands
print a single-line ack — that ack is the only Shale text that should ever
enter the agent's context.

### 3.3 `internal/store` — evidence on disk

```
.shale/
├── README.md                  # auto-generated explainer (what these files are)
├── schema-version             # "0"
├── <session-id>.yaml          # finalized shale files (committed) — see 03-shale-spec.md
├── transcripts/<session-id>.md  # redacted transcript (committed iff privacy=full|redacted)
└── local/                     # GITIGNORED: raw capture JSONL, drafts, adapter state
```

- Finalized shale files are **append-only**: `finalize` never rewrites a
  previously committed shale file (new session → new file). This keeps the audit
  trail honest and diffs clean.
- **Repo-growth discipline (ADR D3a):** shale YAMLs are 2–5 KB — negligible
  at any PR volume. The weight risk is transcripts, so committed transcripts
  are **prompts-only by default**: user prompts + timestamps + per-turn
  one-line summaries, never tool outputs or agent reasoning (~3 KB/session).
  The full raw session stays in gitignored `local/`. `shale gc` (MVP 3)
  prunes shale files for merged PRs past a retention window — history retains
  them for audit; the working tree stays lean.
- **Redaction** runs before anything is written outside `local/`: built-in
  secret patterns (the gitleaks default regex set, vendored) over prompts,
  transcripts, and command lines. Privacy modes: `full`, `redacted` (default),
  `hash-only` (shale carries prompt `sha256` + first-8-words summary only).
- Transcript hash (`sha256` of the redacted transcript file) is embedded in the
  shale YAML → the card can prove the transcript wasn't edited after the fact.
  This is the tamper-evidence story for v1; Sigstore co-signing is the
  hosted-tier upgrade, not duplicated here.

### 3.4 `internal/render` + `internal/forge` — the card

- Render is a pure function: `(shale, prDiff, options) → markdown` — fully
  unit-testable with golden files.
- **`internal/forge` is a driver interface** (the Problem-1 answer):

  ```go
  type Forge interface {
      PRFiles(ctx, pr) ([]ChangedFile, error)        // the diff, via API
      FileContent(ctx, ref, path) ([]byte, error)     // .shale/* at head SHA
      PRCommits(ctx, pr) ([]Commit, error)            // for tamper detection
      UpsertComment(ctx, pr, marker, body) error      // edit-in-place by hidden marker
      SetStatus(ctx, headSHA, state, summary) error   // Check Run / commit status / MR status
  }
  ```

  Drivers: `github` (MVP 1: REST, Check Runs), `gitlab` (MVP 2: MR notes +
  pipeline status), `bitbucket` (backlog). All evidence/diff acquisition is
  **API-based — no code checkout** — which makes the same command work in any
  CI and makes fork PRs safe (§3.7).
- **Card size discipline** (the huge-file-list answer): the changed-files table
  caps at 20 rows; beyond that, group by top-level directory with per-group
  counts and coverage stats, full table inside a `<details>` block. Sensitive
  paths and no-evidence files always surface above the fold regardless of count.
- **Tamper flags** (rendered, never silent — see §5): transcript-hash mismatch;
  shale file modified by a later commit within the PR than the one that
  introduced it (`PRCommits` diff walk).
- **Upsert semantics:** find prior comment by hidden marker
  (`<!-- shale-card -->`), edit in place; one Check Run/status per head SHA.
  Never spam.
- File classification for the "sensitive path" flags: small built-in list
  (dependency manifests, CI/CD config, auth/crypto path globs, IaC), overridable
  later by `.shale/policy.yaml` (MVP 3).

### 3.5 `internal/conformance` — intent↔diff mapping (MVP 3)

Embeds `grove/pkg/grove` (in-process, no daemon — same rule as the rest of the
family). Pipeline:

1. `grove.Query(intent text)` → expected symbol set → expected file set.
2. Expand via `grove.Impact`/`Deps` to the legitimate blast radius.
3. Changed files ∈ blast radius → `in scope`; otherwise → `out of scope` with
   the reason ("not reachable from any symbol matching the stated intent").
4. Degrade gracefully: no Grove index → fall back to MVP 1 behavior
   (session-evidence matching only). Conformance must be additive, never a
   setup requirement.

This is the differentiating feature. It ships third because it's only credible
once capture coverage (MVP 1) and cross-agent breadth (MVP 2) exist.

### 3.6 `action/` — the composite GitHub Action

```yaml
# .github/workflows/shale.yml (written by `shale init`)
name: shale
on:
  pull_request_target:          # write-capable token for fork PRs — safe because
    types: [opened, synchronize, reopened]   # we never check out PR code (§3.7)
permissions:
  contents: read
  pull-requests: write
  checks: write
jobs:
  card:
    runs-on: ubuntu-latest
    steps:
      - uses: provasign/shale-action@v1   # no actions/checkout — everything via API
```

The action downloads the release binary matching its own pinned version +
checksum (supply-chain hygiene: the action tag, binary version, and sha256 are
locked together per release). Marketplace listing is part of MVP 1 launch.

### 3.7 CI-agnostic render contract + the fork-PR model

**Environment contract** — `shale render` runs in any CI that can execute a
binary and provide:

| Variable | Meaning |
|---|---|
| `SHALE_FORGE` | `github` (default) \| `gitlab` |
| `SHALE_TOKEN` | API token (falls back to `GITHUB_TOKEN` / `CI_JOB_TOKEN`) |
| `SHALE_REPO` | `owner/repo` (auto-detected on GitHub Actions / GitLab CI) |
| `SHALE_PR` | PR/MR number (auto-detected from the CI event payload when present) |

Jenkins example (documented in MVP 2 with a copy-paste `Jenkinsfile` stage):

```groovy
sh 'curl -fsSL https://get.shale.dev | bash'
sh 'shale render --pr ${env.CHANGE_ID}'   // CHANGE_ID set by the GitHub/GitLab branch source plugin
```

**Why no checkout, and how fork PRs work (Problem 3, precisely):**

1. The contributor forks. The fork already carries `.shale/` scaffold, the
   workflow file, steering prompt, and repo-level hook config (committed
   upstream). If `shale` is on the contributor's `PATH` and their agent trusts
   repo hooks, capture starts automatically; otherwise the hook silently no-ops
   and the steering + git fallback still produce honest evidence. Contributors
   may run `shale init --global` for machine-wide capture, but there is **no
   upstream registration, no connect step, no token exchange** — committed
   evidence files are why (ADR D3).
2. They push to their fork; shale files ride along; they open a PR to upstream.
3. On GitHub, fork-PR `pull_request` events get a **read-only** `GITHUB_TOKEN`
   — it cannot post comments or Check Runs. We therefore use
   `pull_request_target`, which runs with write permissions in the base-repo
   context. The classic danger of `_target` is executing untrusted PR code with
   a privileged token; Shale eliminates it structurally: **the job never
   checks out or executes anything from the PR.** It fetches the PR's file
   list and the `.shale/*` blobs at the head SHA via the API, renders
   markdown, posts. Untrusted data is *data*, never code.
4. Because shale content is attacker-controlled data entering a privileged
   comment, render sanitizes it: HTML-escape, neutralize `@`-mentions and
   `#`-issue refs, strip link targets to plain text. (Tested with hostile
   fixtures.)
5. Contributor without Shale → standard "no shale" nudge. Identical UX for
   fork and same-repo PRs — this zero-config fork story is a deliberate
   simplification vs. a server-backed model and a core selling point.

On GitLab, the equivalent is an MR pipeline job posting an MR note via the
`gitlab` driver; fork MR token rules differ and are handled inside the driver
(MVP 2).

## 4. Data flow details and edge cases

- **Multiple sessions per branch:** each session is its own shale file; the
  card aggregates (sessions count, union of file touches, all intents listed).
- **Agent without a hook adapter** (Copilot, anything new): `shale intent`
  opens the session JSONL itself; `shale done` closes it; finalize fills
  `files[]` from `git diff` over the intent→done window with `via: git`. The
  card renders these rows with a distinct mark so reviewers can tell
  hook-verified evidence from git-derived evidence.
- **Rebase/squash:** shale files are committed files — they survive rebase and
  squash *with* the code. (This is why git notes lost for v1: notes bind to
  SHAs, which rebase invalidates. ADR D3.)
- **Force-push / amended history:** files still travel; stale session→file
  claims are reconciled by the render step against the actual PR diff (a file
  in the diff with no session evidence is flagged; a session claim for a file
  not in the diff is simply dropped from the card).
- **Fork PRs:** committed files work from forks with zero extra setup — this is
  the single biggest simplification vs. a server-backed model. Full
  mechanics, including the `pull_request_target` / no-checkout token model: §3.7.
- **Monorepos:** shale files live at repo root; `file_touch` paths are repo-relative.
  Per-directory scoping is a non-goal until users ask.
- **Windows:** hooks and paths must be tested on Windows from MVP 1 (Claude
  Code hook commands run through cmd/powershell; use forward-slash-insensitive
  path handling).

## 5. Security and privacy posture

### 5.0 Threat model — be honest about it (ADR D11)

Committed shale files are **tamper-evident, not tamper-proof**:

| Threat | v1 answer |
|---|---|
| Transcript edited after finalize | `transcript.sha256` mismatch → ⚠️ flag on the card |
| Shale YAML hand-edited inside the PR | Detected via `PRCommits`: shale modified after its introducing commit → ⚠️ "shale edited after capture" flag; the edit is also plainly visible in the PR diff (evidence is reviewable — a feature) |
| Shale edited in a later PR | Append-only rule violation; git history shows it; render flags shale files whose file was modified post-introduction |
| **Wholesale fabrication** (malicious author crafts fake shale offline) | **Out of scope for v1, explicitly.** Shale's v1 claim is "what the toolchain recorded for an honest-but-busy team," not insider-attack resistance. Defeating fabrication requires an independent witness — Sigstore/Rekor co-signing and server-side verification — which is precisely a future hosted enterprise tier. The docs and the card never overclaim. |

This tiering is deliberate: it keeps v1 at zero servers while giving the
enterprise version a real reason to exist.

- Prompts can contain secrets and proprietary context → redaction is mandatory
  before anything leaves `local/`; privacy mode is asked at init, defaulting to
  `redacted`; `hash-only` exists for the paranoid org.
- The CLI makes **no network calls** on the laptop. The only network surface is
  `shale render` in CI talking to the forge API with the pipeline's own
  token. There is no telemetry in v1 (a decision, not an oversight — ADR D6).
- Shale content is treated as **untrusted input** wherever it enters a
  privileged surface (the card comment): HTML-escape, neutralize mentions/refs,
  hostile fixtures in the test suite (§3.7).
- Hook handlers run inside developer machines on untrusted input (agent
  payloads): parse defensively, never eval, never write outside `.shale/`
  and the agent's own config files (init only).
- The binary release pipeline must produce checksummed, (goreleaser-) signed
  artifacts from day one — an evidence tool with a sloppy supply chain is dead
  on arrival with the security persona.

## 6. Bridge to a hosted notary (design constraint, not v1 work)

A future `shale sync` (or server-side importer) uploads finalized shale
to a hosted notary, which co-signs it (Sigstore/Fulcio), stores it
org-wide, and applies policy. Constraint on Shale today: **the shale YAML
must be self-contained and schema-versioned** so server-side ingestion never
needs the laptop again. Nothing else about that future tier leaks into this codebase.
