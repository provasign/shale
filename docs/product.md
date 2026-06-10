# Shale — product, architecture, and design

The single reference for what Shale does, how it's built, and why it's built
that way. The evidence file format lives separately in
[shale-spec.md](shale-spec.md) — that's the contract third parties implement.
User-facing walkthroughs: [getting-started.md](getting-started.md) ·
[troubleshooting.md](troubleshooting.md).

---

## §1 What Shale is

Shale records what an AI coding agent was asked to do, what it touched, and
what checks it ran — then renders that evidence as a card on the pull request.

It exists because agent-authored PRs arrive as bare diffs: the prompt, the
session trail, and the local check history that would explain the change are
on the author's laptop, invisible to the reviewer. Shale makes that evidence
travel with the code.

**What Shale is not — by design, not omission:**

- **Not a quality gate.** The card is advisory; the CI check is never
  `failure`. Trust products die when they add friction before proving value.
- **Not an LLM reviewer.** Shale records and renders; it never judges the
  code. No model calls anywhere in the product.
- **Not a hosted service.** No account, server, GitHub App, token paste, or
  telemetry. The laptop half is verifiably offline; only the CI half talks to
  the forge API, using the repo's own `GITHUB_TOKEN`.

## §2 The five-minute promise

Setup is the product's first feature. One person runs:

```sh
brew install provasign/shale/shale
cd your-repo && shale init
git add . && git commit -m "chore: enable shale" && git push
```

Everything `shale init` writes is **committed**, so the whole team and all
their agents are wired on `git clone` — there is no per-developer setup:

| Written | Purpose |
|---|---|
| Steering block in `CLAUDE.md`, `AGENTS.md`, `.cursorrules`, … | Tells the agent to declare `shale intent` / `shale done` — works with any agent that can run a shell command |
| Repo hook config (`.claude/settings.json`, `.github/hooks/shale.json`, `.cursor/hooks.json`, `.codex/hooks.json`) | Streams file touches, commands, prompts into the session record for hook-capable agents |
| `.shale/` scaffold | Committed, schema-versioned, redacted evidence store; `local/` is gitignored working state |
| `.github/workflows/shale.yml` | Renders the card on PRs — API only, never a checkout |
| `.git/hooks/pre-push` (local) | `shale finalize --auto-commit` — folds and commits evidence before push, fail-open |

Any change that adds a setup step, an account, a server round-trip, or a
question without a default is wrong by definition.

## §3 The card

The card answers a reviewer's first questions before the diff: *what was the
agent asked to do, what did it touch, what ran, and what's unaccounted for?*

Sections, top to bottom:

1. **Header** — session count, tool, model, token/cost/iteration/duration
   summary.
2. **Tamper flags** (when present) — transcript hash mismatch, evidence edited
   after capture. Rendered prominently as blockquote warnings.
3. **Intent** — the agent's declared goal (title + body), with declaration
   time and session ID. (When a session carries a transcript — see §6 — the
   card links it with its SHA-256 pin; current builds don't emit them.)
4. **Completion** — the agent's closing self-report.
5. **Changed files** — every file in the PR diff with its evidence state:
   `✅ <session>` hook-verified · `◐ <session>` git-derived ("changed during
   session — not hook-verified") · `—` no evidence. Sensitive paths
   (dependency manifests, CI config, IaC, auth/crypto paths) are bolded and,
   on large PRs, kept above the fold while the rest collapses into a
   directory-grouped `<details>` block.
6. **Checks recorded locally** — commands classified as tests/scans, with
   exit codes and times, footnoted *"Advisory — CI is authoritative."*

Two invariants shape everything above:

- **Absence is explicit, never silent.** A PR with no evidence gets a clear
  "No shale for this PR" nudge (posted at most once). A session without intent
  renders "No intent declared." Files without evidence are listed, not
  dropped.
- **All shale-derived text is attacker-controlled data.** Everything passes
  through a sanitizer (HTML escaped, `@mentions`/`#refs` neutralized with
  zero-width spaces, link syntax broken, control characters stripped) before
  entering the card. Hostile evidence degrades to warnings; it never breaks
  rendering or pings people.

## §4 Capture: three tiers and the activation model

Intent is **agent-declared, not hook-inferred** — hooks can't know *why* an
agent edits a file, and inferring intent from the first user prompt produces
garbage. The agent declares intent at the moment of maximum clarity (after
reading the request, before touching files). That's the semantic tier, and it
needs nothing but a shell:

| Tier | Mechanism | Works with |
|---|---|---|
| **Semantic** | Steering prompt → `shale intent` / `shale done` CLI calls | Every agent (universal) |
| **Hook-verified** | Agent hooks pipe event JSON to `shale capture <adapter>` | Claude Code today; Cursor/Codex/Copilot configs are written and inert until their adapters ship |
| **Git fallback** | `shale finalize` derives the file list from git over the intent→done window | Anything without hook events, marked `via: git` |

Hooks **upgrade fidelity** (per-file verification, prompt transcripts,
command exit codes); they are never required. Partial hook coverage merges
per-path with the git fallback — hook evidence wins for files it saw.

**Why CLI, not MCP:** MCP tool results pin to the agent's context window for
the whole session; CLI output flows through the normal compression path. An
earlier internal system measured MCP integration at a third of session tokens
and reversed course. MCP also needs per-agent server registration and approval
ceremonies; the CLI needs only PATH. Every agent that can edit files can run
a shell command — steering + CLI is the only universal channel.

**Per-agent hook wiring** (verified against vendor docs 2026-06; recorded
fixtures are required before any parser ships):

| Agent | Repo config | Trust gate | Notes |
|---|---|---|---|
| Claude Code | `.claude/settings.json` | none documented | Implemented. Also read by VS Code Copilot (`chat.hookFilesLocations` default) |
| VS Code Copilot | `.github/hooks/*.json` + Claude-format files | workspace trust | Hooks feature is Preview |
| Copilot CLI / cloud agent | `.github/hooks/*.json` | none documented | camelCase events |
| Cursor | `.cursor/hooks.json` | workspace trust | payload carries `transcript_path` |
| Codex CLI | `.codex/hooks.json` | explicit `/hooks` review | partial tool interception; edits surface as `apply_patch` |

All repo hook commands are **self-guarding**
(`command -v shale >/dev/null 2>&1 && shale capture <agent> || true`): a
contributor without the binary sees zero errors, and the hook activates the
moment they install it. Hook config is written for **all** agents,
unconditionally — the person running `init` can't know which agents future
contributors use, and inert config costs nothing. `shale init --global`
exists for individuals who want machine-wide capture (detection-gated,
unguarded commands).

Activation is repo-level twice over: the steering block lives in the repo,
and `shale capture` no-ops unless the repo has a `.shale/` directory.

## §5 Architecture

Two halves, two trust domains, one committed directory between them:

```text
LAPTOP (offline)                          CI (forge API only)
agent hooks ─→ shale capture ─┐
steering    ─→ shale intent   ├→ .shale/local/<session>.jsonl   (events)
               shale done    ─┘
                    git push → pre-push → shale finalize
                                  │  redact → fold → commit
                                  ▼
                        .shale/<session>.yaml + transcripts/   (committed)
                                  ▼
                   workflow → shale render → forge API
                       fetch evidence + diff (NO checkout)
                       verify transcript hashes (tamper-evidence)
                       render card → upsert PR comment + neutral check
```

Key components map 1:1 to packages:

- `internal/capture` — hook payload parsers; total functions, fail-open,
  fixture-tested per agent.
- `internal/store` — event log + shale YAML read/write, schema versioning,
  redaction before persistence.
- `internal/fold` — finalize: events → shale file + prompts-only transcript,
  git-fallback file derivation, path normalization (rejects absolute/escaping
  paths).
- `internal/render` — pure function from shale files + PR diff to markdown;
  golden-file tested, sanitizer enforced.
- `internal/forge` — the forge driver interface (GitHub today; the renderer
  never imports a forge SDK). CI-agnostic core: the GitHub Action is
  packaging, not architecture.
- `internal/prcard` — CI orchestrator: fetch, verify, render, post.

**The no-checkout rule is the security model.** The render workflow runs on
`pull_request_target` (so fork PRs get cards with a write-capable token) and
is safe *only because it never checks out PR code* — everything arrives
through the API as data. Adding a checkout step to that workflow is the one
forbidden change; `shale doctor` screams about it.

## §6 Privacy and the trust boundary

- **Redaction before persistence.** Agent-authored text (intent, notes,
  commands) passes a redaction pass before anything is written, in one of
  three committed modes: `full` · `redacted` (default — secrets/tokens
  stripped) · `hash-only` (hashes only).
- **Raw prompt transcripts are feature-flagged OFF** (`transcriptsEnabled`
  in `internal/fold`, deliberately not exposed via config, flag, or env).
  The capability exists — prompts-only, redacted, SHA-256-pinned transcripts
  committed beside the evidence — but pattern-based redaction is not yet
  trustworthy for free-form human text (unstructured passwords, PII, pasted
  proprietary material, venting), and the regulatory position on persisting
  developers' raw prompts is unsettled. Until both change, prompts live only
  in gitignored `.shale/local/`; committed evidence carries the prompt count
  and intent title hash, so the schema is unchanged whenever the flag flips.
  The render/verify path for transcripts is kept working for historical
  evidence.
- **Tamper-evident, not tamper-proof — and the card says so.** Transcript
  hashes and after-capture edits are verified at render time and flagged.
  Wholesale fabrication by a malicious author is explicitly out of scope for
  v1: defeating it requires an independent witness (Sigstore/Rekor
  co-signing, server-side verification) — future hosted-tier work. The card
  never overclaims; honest provenance labels (`via: hook` vs `via: git`,
  "exit unknown") are load-bearing.
- **No telemetry, no accounts, no laptop network calls.** Enforced by test.

## §7 Settled design decisions

Distilled decision records. Don't relitigate these in PRs; if reality
contradicts one, open an issue describing the contradiction.

| # | Decision | Why |
|---|---|---|
| D1 | Apache-2.0 | Evidence formats win by adoption; patent grant matters for enterprise |
| D2 | Go everywhere; composite (not TypeScript) Action | One toolchain, static binaries, no Node in CI |
| D3 | Evidence = committed files in `.shale/` | Survives forks/clones with zero infrastructure; git-notes mode later for commit-noise-averse teams |
| D4 | Steering + CLI capture; hooks as enhancement; MCP rejected | Universality + token economics (§4) |
| D5 | Advisory and fail-open everywhere | A trust product must not add friction before proving value; first enforcement knob arrives later, opt-in |
| D7 | Shale does not run gates | Recording and judging must not mix; CI is authoritative |
| D8 | Session/file granularity, not line-level attribution | Line attribution is noise under rebase/refactor; file × session is what reviewers act on |
| D10 | CI-agnostic core; forge drivers behind an interface | GitLab/Jenkins reachable without re-architecting |
| D11 | Tamper-evident v1; tamper-proof is hosted-tier work | Honest engineering line (§6) |
| D12 | `pull_request_target` + zero checkout | Fork PRs get cards without the classic privilege escalation (§5) |

## §8 Contributor invariants

The short list that every change must preserve:

1. **The 5-minute promise** — no new setup steps, accounts, or questions
   without defaults.
2. **Fail-open everywhere** — capture exits 0 on every error; finalize never
   blocks a push; render never fails a check.
3. **No network from laptop code paths** — only CI render talks to the forge.
4. **Redaction before persistence** — seeded-secret tests required.
5. **Append-only evidence** — never rewrite a finalized shale file.
6. **Fixtures over mocks** for hook payloads — record real agent payloads
   into `testdata/`, note the agent version; hook-API drift is an expected
   maintenance cost.
7. **Golden files for all card rendering**; `golangci-lint` clean; LF line
   endings (`.gitattributes` enforces).
