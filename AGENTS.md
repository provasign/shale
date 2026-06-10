# Instructions for the implementing agent

You are building **Shale** from scratch in this repository. The complete
specification lives in `docs/`. Read in this order before writing any code:

1. `docs/04-implementation-plan.md` — what to build, in what order, with
   acceptance criteria. Work milestone by milestone; do not skip ahead.
2. `docs/02-architecture.md` — components, data flow, edge cases.
3. `docs/03-shale-spec.md` — the shale file format (the contract; implement
   it exactly, including the rules in §3).
4. `docs/01-product.md` — UX flows and card mockups (golden-file targets).
5. `docs/05-decisions.md` — settled decisions. Do not relitigate; if reality
   contradicts one, stop and report instead of diverging.

## Ground rules

- **Language:** Go 1.26+, module `github.com/provasign/shale`. Add it to the
  workspace `go.work` one directory up. No CGO, no SQLite, no Node toolchain.
- **The 5-minute promise is the product.** Any change that adds a setup step,
  an account, a server, or a question without a default is wrong by definition.
- **Fail-open everywhere** (hooks, finalize, render). A Shale bug must never
  break a user's agent, push, or CI.
- **No network calls from laptop-side code paths.** Only `shale render` in CI
  talks to the GitHub API. Write a test that enforces this.
- **Redaction before persistence.** Nothing from a prompt/transcript/command
  reaches a committed file before the redaction pass. Seeded-secret tests are
  required, not optional.
- **Append-only shale files.** Never rewrite a finalized shale file.
- **Fixtures over mocks** for agent hook payloads: record real payloads into
  `testdata/`, note the agent version they came from, and treat hook-API drift
  as an expected ongoing cost (check current vendor docs at implementation
  time — Claude Code / Cursor / Codex hook schemas move).
- **Conventions:** Makefile with `build/test/lint/install`, table-driven tests,
  golden files for all card rendering, `golangci-lint` clean.
- Commit in small, reviewable units per workstream item (A1, A2, …) with the
  item ID in the commit message.

## Definition of done per milestone

A milestone is done when every acceptance criterion in
`docs/04-implementation-plan.md` for it passes as an automated test (or, where
genuinely manual — e.g. the timed activation run — is performed and its result
recorded in the repo). Then stop and report before starting the next milestone.
