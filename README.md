# Shale

> **Every agent PR should explain itself.**
>
> Shale captures what an AI coding agent was asked to do, what it touched, and
> what checks it ran, then renders that evidence as a pull-request card.
> Five minutes to set up. No account. No server. Works across coding agents.

Shale is the primary open-source product in the Provasign org. Prism is the
secondary tool for graph-ranked agent context. Grove is the standalone graph
engine for teams that want the code graph directly.

**License:** Apache-2.0.

---

## Why Shale Exists

AI coding agents are now producing real PR volume. Reviewers still get the same
old diff, often without the prompt, session trail, or local check history that
would explain whether the agent did the right thing.

Shale makes that missing evidence travel with the code.

It is not a quality gate, LLM reviewer, or hosted attestation service. It is a
local-first recorder and PR renderer:

- **Intent:** what the agent was asked to do.
- **Evidence:** files seen in the agent session, commands/checks observed, model
  and token metadata when available.
- **Gaps:** changed files with no session evidence, sensitive paths, or
  hook-limited coverage.
- **Card:** a neutral PR comment/check that reviewers can read before the diff.

The default posture is advisory and fail-open. A Shale bug must not block your
agent, push, CI, or merge.

## Quickstart

```sh
brew tap provasign/shale
brew install shale
cd your-repo
shale init
git add . && git commit -m "chore: enable shale"
```

If Homebrew asks you to trust the tap:

```sh
brew trust --formula provasign/shale/shale
brew install shale
```

If Homebrew is not available, download the latest release from
<https://github.com/provasign/shale/releases/latest> and put the `shale` binary
on your `PATH`.

## What `shale init` Writes

`shale init` wires the repo for the lowest-friction path:

- agent steering instructions in files such as `AGENTS.md`, `CLAUDE.md`,
  `.cursorrules`, and `.github/copilot-instructions.md`
- repo-level hook config for agents that support hooks
- `.shale/` storage for redacted, schema-versioned evidence
- a GitHub Action that renders the Shale card on pull requests
- a local pre-push hook that finalizes evidence before push

Repo hooks are guarded. If a teammate has not installed Shale, the generated
hook command silently no-ops. Developers who want machine-wide capture can run
`shale init --global`.

## How It Works

```text
agent receives task
  -> steering asks the agent to call shale intent
  -> hooks and CLI calls record session evidence locally
  -> agent calls shale done
  -> git push runs shale finalize
  -> CI runs shale render
  -> PR gets a Shale card
```

The evidence is committed under `.shale/` after redaction, so fork PRs and
same-repo PRs work the same way. The GitHub Action uses the repo's
`GITHUB_TOKEN`; there is no signup, token paste, GitHub App, or server.

## The PR Card

A Shale card answers the reviewer's first questions:

- What was the agent asked to do?
- Which agent/model/session produced this change?
- Which changed files were seen in session evidence?
- Which files have no evidence?
- What local checks did the agent run?
- Are there sensitive path or tamper warnings?

If a PR has no Shale evidence, Shale still renders a clear no-evidence card
instead of silently disappearing.

## Project Lineup

| Project | Role | Status |
|---|---|---|
| **Shale** | Primary product: agent PR evidence and intent cards | Active, Apache-2.0 |
| **Prism** | Secondary product: graph-ranked context delivery for agents | Active, MIT |
| **Grove** | Code graph engine for direct graph/index use and embedded tools | Active, MIT |

Shale may use Prism or Grove-backed conformance features over time, but the
core product remains useful without a hosted backend.

## Repository Layout

```text
cmd/shale/              CLI entry point
internal/capture/       agent hook payload parsers
internal/store/         .shale read/write, schema versioning, redaction
internal/render/        PR card rendering
internal/forge/         forge API drivers
action/                 composite GitHub Action
docs/                   product, architecture, spec, implementation plan
```

## For Implementing Agents

Read in this order:

1. `AGENTS.md`
2. `docs/04-implementation-plan.md`
3. `docs/02-architecture.md`
4. `docs/03-shale-spec.md`
5. `docs/01-product.md`
6. `docs/05-decisions.md`

The short version: preserve the 5-minute setup promise, fail open everywhere,
redact before persistence, keep `.shale/` append-only, and prefer real hook
fixtures over mocks.
