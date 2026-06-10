# Shale — Implementation plan (MVP 1 → 3)

This is the build order. Each milestone has a demo-able exit criterion; do not
start the next milestone until the current one's acceptance tests pass. The
implementing agent should treat acceptance criteria as the definition of done
and write them as automated tests where possible.

Conventions: Go 1.26 module `github.com/provasign/shale` (add to root
`go.work`), Makefile targets `build/test/lint/install`, table-driven tests,
golden files for rendering, recorded hook payloads in `testdata/`.

---

## MVP 1 — "Shale on the PR" (the 5-minute promise)

**Exit demo:** on a fresh clone of a sample repo, a person who has never seen
this project runs 2 commands, has their agent write a change, pushes, opens a
PR, and sees a correct Shale card — in under 5 minutes, with no account and
no server. The semantic tier (`shale intent` / `shale done` via steering
prompt, ADR D4) works with **any agent**; Claude Code sessions additionally
show hook-verified file evidence. A second PR from a contributor *without*
Shale gets the "no shale" nudge comment exactly once.

### Workstream A — skeleton & store
- [ ] A1. Repo scaffold: Go module, Makefile, golangci-lint, CI (test + lint +
      goreleaser dry-run), Apache-2.0 LICENSE.
- [ ] A2. `internal/store`: shale v0 structs (mirror `docs/03-shale-spec.md`
      exactly), YAML read/write, `schema-version` file, append-only guard,
      ULID generation. Unit tests round-trip the spec example verbatim.
- [ ] A3. Redaction engine: vendor the default gitleaks regex set; apply to
      prompts/transcripts/commands; counter only (never log matches). Privacy
      modes `full|redacted|hash-only` implemented at finalize time.
      Test: seeded secrets in fixtures never appear in any output file.

### Workstream B — capture (semantic tier universal; hooks for Claude Code)
- [ ] B1. `shale intent "<title>" [--body "..."]` and `shale done [--note]
      [--tokens-in N] [--tokens-out N] [--model M] [--iterations N]`:
      agent-called CLI commands (steered, ADR D4). Open/append the session
      JSONL under `.shale/local/` (create the session if no hook adapter has);
      `done` records the completion block; `cost_usd` computed at finalize
      from the pinned pricing table. **Hard requirements:** <50 ms, exit 0 on
      every error, single-line stdout ack (the only Shale text that should
      enter the agent's context).
- [ ] B2. `shale capture claude-code`: parse hook stdin JSON for
      `UserPromptSubmit`, `PostToolUse` (Edit/Write/MultiEdit + Bash), `Stop`,
      `SessionStart`. Normalize to events; append JSONL under
      `.shale/local/`. **Hard requirements:** <50 ms, exit 0 on every error
      (errors to `.shale/local/capture.log`). Fixtures: real recorded
      payloads (record them during implementation; check current Claude Code
      hooks docs — the API surface moves).
- [ ] B3. `shale finalize`: fold JSONL → shale YAML + redacted transcript +
      hashes; idempotent (re-run without new events = no-op); append-only.
      **Git fallback (ADR D4 tier 3):** when a session has intent/done but no
      hook events, fill `files[]` from `git diff` over the session window,
      marked `via: git`.
- [ ] B4. Pre-push hook (installed by init): runs finalize; auto-commits
      `.shale/` changes as `chore(shale): session evidence` (ADR D3);
      never blocks the push on its own failure (fail-open, warn on stderr).

### Workstream C — init & doctor
- [ ] C1. `shale init`: write the steering prompt (marker-fenced, idempotent,
      removable — architecture §3.2a) into every detected agent instruction
      file (CLAUDE.md, AGENTS.md, .cursorrules, .windsurfrules, .clinerules,
      `.github/copilot-instructions.md`, GEMINI.md, `.kiro/steering/`);
      write guarded repo-level hook config for Claude Code, Copilot, Cursor,
      and Codex (additive/idempotent, preserving existing config); scaffold
      `.shale/` (+ README + .gitignore for `local/`); write
      `.github/workflows/shale.yml`; support `--global` for detection-gated
      machine-wide hooks; privacy defaults to redacted.
- [ ] C2. `shale doctor`: checks steering prompt present in at least one
      instruction file, hooks present, events flowing (last capture
      timestamp), workflow file present, finalize freshness; one actionable
      line per problem.
- [ ] C3. Windows + macOS + Linux: init/capture/finalize integration test in CI
      on all three (the hook command path quoting on Windows is the known trap).

### Workstream D — render & Action
- [ ] D1. `internal/render`: pure function (shale + PR file list → markdown
      card per `docs/01-product.md` §3.3, including coverage-gap table,
      sensitive-path flags, and the 20-row cap with directory grouping +
      `<details>` collapse). Golden-file tests for: single session,
      multi-session, no shale, partial coverage, hash-only privacy,
      200-file PR.
- [ ] D2. `internal/forge`: driver interface per architecture §3.4; `github`
      driver (PR files, file contents at head SHA, PR commits, comment upsert
      by hidden marker, neutral Check Run). **All API-based — no checkout.**
      Respect `SHALE_TOKEN`/`GITHUB_TOKEN`; rate-limit-aware retries.
- [ ] D3. `shale render --pr` (CI entry, env contract per architecture §3.7,
      auto-detect repo/PR on GitHub Actions) + `--local` (terminal preview).
- [ ] D4. `action/`: composite action downloading the checksum-pinned release
      binary; workflow template uses `pull_request_target` with **no checkout
      step**; `provasign/shale-action@v1` tagging locked to binary version +
      sha256. End-to-end test on a real sandbox repo **including a fork PR**
      (card must post with full evidence from the fork).
- [ ] D5. "No shale" nudge: posted once per PR (marker-deduped), exact copy
      from `docs/01-product.md` §3.4.
- [ ] D6. Untrusted-content hardening: HTML-escape shale content in the card,
      neutralize `@`-mentions / `#`-refs / link targets; hostile fixture suite
      (shale files crafted to inject markdown/mentions).
- [ ] D7. Tamper flags: transcript-hash verification + "shale edited after
      its introducing commit" detection via PR commit walk; golden cards for
      both warning states.

### Workstream E — release & docs
- [ ] E1. goreleaser: darwin/linux/windows × amd64/arm64, checksums, signed
      artifacts; `install.sh` + Homebrew tap.
- [ ] E2. README quickstart that *is* the 5-minute walkthrough; sample repo
      with a scripted demo; GitHub Marketplace listing for the Action.
- [ ] E3. Timed activation test: a fresh person (or scripted run) from
      `brew install` to rendered card — must be ≤ 5 minutes; record the timing
      in the repo.

**MVP 1 acceptance (all must hold):**
1. Two commands from zero to working setup; init idempotent; nothing
   overwritten without consent; steering prompt lands in every detected
   agent instruction file.
2. Agent session with `shale intent` + `shale done` (any agent, via steering)
   → committed shale YAML that validates against the spec example, with
   secrets provably redacted; committed transcripts are prompts-only (no tool
   output ever). Claude Code sessions additionally carry hook-verified
   `files[]`/`commands[]`; sessions without hooks carry `via: git` files.
3. PR card renders correctly for the golden scenarios (incl. 200-file PR and
   both tamper-flag states); absence is always explicit; Check Run is never
   failing.
4. Laptop half makes zero network calls; the Action does **no code checkout**;
   a fork PR posts a full card; hostile shale fixtures cannot inject into
   the card.
5. Works on macOS/Linux/Windows in CI.

---

## MVP 2 — "Cross-agent + the open spec" (the neutrality moat)

**Exit demo:** the same repo gets PRs from Claude Code, Cursor, and Codex CLI
sessions; all three render identical-quality cards with hook-verified
evidence (the semantic tier already worked for all of them in MVP 1 — this
milestone closes the hook-enhancement gap). `shale verify` validates any
shale against the published JSON Schema. A third-party tool could emit a
valid shale using only the `spec/` directory.

- [ ] F1. Cursor hook adapter (hooks API; fixture-recorded payloads).
- [ ] F2. Codex CLI hook adapter (hook/notify config; session-file tailing
      fallback).
- [ ] F3. Gemini CLI hook adapter; adapter conformance test harness (one
      fixture suite all adapters must pass).
- [ ] F4. Generic fallback: `shale wrap -- <cmd>` PTY wrapper + `shale note`.
- [ ] G1. Spec v1: `spec/shale.v1.schema.json`, conformance fixtures,
      `shale verify`, versioning policy doc. Publish + announce as an open
      spec (blog post / spec README aimed at other tool authors).
- [ ] G2. `commands[]` classification hardening → "Checks recorded locally"
      card section with test/lint/scan grouping (recording only — still no gate
      execution).
- [ ] G3. Card v2: multi-agent aggregation, persona-aware section ordering
      (security-first when sensitive paths changed), collapsible long sections.
- [ ] G4. `shale status` (laptop-side: what will be in the next shale).
- [ ] G5. **CI breadth (enterprise reality):** `gitlab` forge driver (MR note
      upsert + pipeline status, fork-MR token handling); documented Jenkins
      recipe (`Jenkinsfile` stage using the env contract) tested against a
      GitHub repo driven from Jenkins; CI auto-detection for GitLab CI env.
- [ ] G6. `transcript.kind: full` opt-in path (default stays prompts-only).
- [ ] H1. Hardening from MVP 1 field reports: hook API drift detection
      (`doctor` warns when an agent's hook schema changed), capture-loss
      telemetry *in the card* (e.g. "session evidence may be incomplete").

**MVP 2 acceptance:** 3+ agents with identical card quality; spec v1 published
with schema + fixtures; an external emitter test (hand-written shale passes
`verify` and renders).

---

## MVP 3 — "Conformance + trust" (the killer feature, the enterprise on-ramp)

**Exit demo:** a PR whose diff includes changes outside the stated intent's
blast radius shows them flagged `out of scope — not reachable from the stated
intent`, computed by Grove locally in CI, with graceful fallback when no index
exists.

- [ ] I1. Embed `grove/pkg/grove`; index at render time in CI (or restore from
      actions/cache); budget: conformance adds ≤ 60 s to the Action on a 5k-file
      repo, else auto-skip with explicit card note.
- [ ] I2. `internal/conformance`: intent text → Query → Impact/Deps blast
      radius → per-file in/out-of-scope classification with one-line reasons.
      Golden tests on fixture repos (the demo scenario above is a fixture).
- [ ] I3. `.shale/policy.yaml`: org-tunable sensitive-path globs and
      (opt-in) `strict: true` — Check Run failure *only* on
      missing-shale-or-out-of-scope, clearly documented as the first
      enforcement knob.
- [ ] I4. Git-notes storage mode (opt-in alternative to committed files, for
      repos that reject evidence commits); Action fetches the notes ref.
- [ ] I4b. `shale gc`: prune shale files/transcripts for PRs merged more than a
      retention window ago (default 180d, configurable in policy.yaml);
      history remains the audit record.
- [ ] I5. Provasign bridge contract test: a fixture proving a finalized shale
      is ingestible without laptop access (self-containedness check — no
      Provasign code here, just the contract).
- [ ] I6. (Stretch) DSSE/Sigstore envelope as an *external wrapper* command for
      teams that want Rekor logging without Provasign.

**MVP 3 acceptance:** conformance demo passes on fixtures; fallback path never
degrades MVP 1 behavior; strict mode is opt-in, default unchanged (advisory).

---

## Sequencing rationale & sizing

| Milestone | Why this order | Rough size (focused agent-weeks) |
|---|---|---|
| MVP 1 | Activation + growth loop first; everything else is moot without installs | 3–4 |
| MVP 2 | Hook-verified depth across agents (semantic tier is already universal from MVP 1) is the defensible position vs. GitHub's Copilot-only tracing; spec makes it a standard, not a tool | 3–4 |
| MVP 3 | Conformance is only credible on top of real capture coverage; it's also the Provasign upsell hook | 4–6 |

## Standing risks the implementer must watch

1. **Hook API drift** (Claude Code/Cursor/Codex change their hook schemas):
   isolate per-adapter, fixture-test, make `doctor` detect drift. Expect to
   patch adapters monthly. Drift only degrades the enhancement tier — intent,
   completion, and the git-fallback file list (ADR D4) keep working.
2. **Steering-prompt compliance** (the agent ignores or forgets the
   `shale intent`/`shale done` instruction): mitigations are prompt placement
   (top-level instruction file, marker-fenced), the one-line ack keeping the
   ceremony cheap, and the card making absence explicit ("no intent declared")
   so non-compliance is visible, not silent. Measure compliance rate in
   dogfooding before launch; Provasign's production experience with the same
   pattern is the existence proof.
3. **Evidence-commit allergy** (teams disliking `chore(shale)` commits):
   measured by feedback; the answer is MVP 3's notes mode — do not redesign
   MVP 1 around it preemptively (ADR D3).
4. **Prompt privacy objections**: defaults already conservative; `hash-only`
   exists; never weaken redaction for card prettiness.
5. **GitHub ships cross-agent tracing natively**: the counter is spec
   neutrality (works on GitLab/Gitea later, works for *local* agents, feeds
   Provasign). If this happens, accelerate G1 and G5 (non-GitHub targets).
6. **`pull_request_target` footguns**: any future contributor adding an
   `actions/checkout` of the PR head to the workflow reintroduces the classic
   privilege-escalation hole. The action README must carry a loud warning, and
   `shale doctor` should flag a checkout step in `.github/workflows/shale.yml`.
7. **Shale forgery objections from security reviewers**: point to the threat
   model (architecture §5.0) — v1 is tamper-evident by design, fabrication
   resistance is the Provasign notary tier. Don't ship half a signing scheme
   in this repo to appease the objection.
