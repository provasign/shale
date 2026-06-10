# Instructions for agents working in this repository

Shale is shipped and in maintenance/feature mode. Before changing code, read:

1. `docs/product.md` — functionality, architecture, and the settled design
   decisions (§7 — do not relitigate; if reality contradicts one, stop and
   report instead of diverging).
2. `docs/shale-spec.md` — the shale file format. It is a public contract;
   implement it exactly.

## Ground rules

- **Language:** Go 1.26+, module `github.com/provasign/shale`. No CGO, no
  SQLite, no Node toolchain.
- **The 5-minute promise is the product.** Any change that adds a setup step,
  an account, a server, or a question without a default is wrong by definition.
- **Fail-open everywhere** (hooks, finalize, render). A Shale bug must never
  break a user's agent, push, or CI.
- **No network calls from laptop-side code paths.** Only `shale render` in CI
  talks to the GitHub API. A test enforces this; keep it passing.
- **Redaction before persistence.** Nothing from a prompt/transcript/command
  reaches a committed file before the redaction pass. Seeded-secret tests are
  required, not optional.
- **Append-only shale files.** Never rewrite a finalized shale file.
- **Fixtures over mocks** for agent hook payloads: record real payloads into
  `testdata/`, note the agent version they came from, and treat hook-API drift
  as an expected ongoing cost (check current vendor docs at implementation
  time — Claude Code / Cursor / Codex hook schemas move).
- **Conventions:** Makefile with `build/test/lint/install`, table-driven tests,
  golden files for all card rendering, `golangci-lint` clean, LF endings.
- Commit in small, reviewable units with clear messages.

## Releases

Cross-compile per platform with the binary named `shale` inside each
`shale_{os}_{arch}.tar.gz` (the Action extracts it by name), publish per-asset
`.sha256` files, tag the release, then move `shale-action`'s `v1` tag and the
action's default `version:` to the new release.
