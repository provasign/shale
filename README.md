# Shale

> **Every agent PR comes with shale.**
> Shale shows reviewers what an AI coding agent was asked to do, what it actually
> changed, and what checks already ran — rendered as a card on the pull request.
> Five minutes to set up. Zero servers. Works with every coding agent.

**License: Apache-2.0** (deliberately permissive — adoption is the moat; see
`docs/05-decisions.md` D1).

---

## The problem

AI coding agents now produce a large share of PR volume. The reviewer, the
security engineer, the risk officer, and the product owner all ask the same
question from different angles:

> **"Did the agent do the right thing? What was it asked, what did it touch,
> and can I trust this diff?"**

Today the answer is scattered or gone: the prompt died with the agent session,
the diff is huge, and CI findings arrive after the fact. GitHub is solving this
*only* for its own Copilot cloud agent (commit → session-log tracing). Local
agents — Claude Code, Cursor, Codex CLI, Gemini CLI, Windsurf — which produce
most agent code today, leave no trail.

## What Shale does

1. **Captures intent from the agent itself.** A steering prompt (written by
   `shale init` into CLAUDE.md / AGENTS.md / .cursorrules /
   copilot-instructions.md) has the agent declare its intent via
   `shale intent` before the first edit and report completion via
   `shale done` — plain CLI calls, no MCP, so it works with **every** agent
   including Copilot (ADR D4). Agent-native hooks, where available, add
   verified file-touch and command evidence automatically; a git-diff fallback
   covers agents without hooks. The user changes nothing about how they work.
2. **Makes the evidence travel with the code.** Evidence is written to
   `.shale/` in the repo (schema-versioned YAML, redacted) and rides along
   with the normal push. No server in the path.
3. **Renders a Shale card on the PR.** `shale render` runs in the user's
   own CI — a one-line GitHub Action for OSS, or the same binary in a
   Jenkins/GitLab CI stage for enterprises — reads the PR diff and the shale files
   via the forge API (no code checkout), and posts a card: intent, agent/model,
   changed files vs. stated intent, locally-recorded check results, and
   explicit gaps ("2 files changed with no session evidence").
4. **Stays advisory.** The card never blocks a merge. Strict mode is a later,
   explicit opt-in. The default posture is fail-open, zero friction.

What Shale deliberately does **not** do:

- It does **not** run quality gates. Scanners (Semgrep, Sonar, gitleaks) already
  ship MCP servers that agents call in-loop; CI remains authoritative. Shale
  *records* what ran and what the result was — it is a recorder and renderer,
  not a gatekeeper.
- It does **not** review code with an LLM. CodeRabbit/Greptile/Graphite own
  that space. Shale is the evidence layer those reviews sit on top of.
- It does **not** require an account, a backend, signing keys, or a GitHub App
  in v1.

## The one-line goal (hold every decision against this)

> **A stranger with an agent-built branch gets a Shale card on their next PR
> within 5 minutes of discovering this project, without creating an account or
> deploying anything.**

## Relationship to Provasign

Shale is the open, lightweight market wedge. Provasign (sibling repo) remains
the enterprise certification platform (Sigstore notarization, org policy,
server-side attestation store, regulated-tier re-runs). The bridge is the
shale format itself: a Provasign server can ingest `.shale/` files and
upgrade them into signed attestations. Shale feeds Provasign; it does not
depend on it. Grove (sibling, MIT) is embedded in MVP 3 for intent↔diff
conformance.

## Repository layout (target)

```
shale/
├── cmd/shale/              # CLI entry point (Go, single static binary)
├── internal/
│   ├── capture/            # agent hook adapters (claudecode, cursor, codex, generic)
│   ├── store/              # .shale/ read/write, schema versioning, redaction
│   ├── render/             # card rendering (markdown) for PR/MR comment
│   ├── forge/              # forge drivers: github (MVP 1), gitlab (MVP 2)
│   └── conformance/        # MVP 3: Grove-backed intent↔diff mapping
├── action/                 # composite GitHub Action (packaging — see ADR D10)
├── spec/                   # shale format JSON Schema + examples (the open spec)
├── docs/
│   ├── 01-product.md       # personas, UX flows, card mockups
│   ├── 02-architecture.md  # components, data flow, language rationale
│   ├── 03-shale-spec.md    # evidence format v0
│   ├── 04-implementation-plan.md  # MVP 1/2/3 task breakdown (start here to build)
│   └── 05-decisions.md     # decision records — do not relitigate these
└── AGENTS.md               # instructions for the implementing agent
```

## Reading order for the implementing agent

1. `AGENTS.md` — ground rules
2. `docs/04-implementation-plan.md` — what to build, in order, with acceptance criteria
3. `docs/02-architecture.md` + `docs/03-shale-spec.md` — how
4. `docs/01-product.md` — why / UX north star
5. `docs/05-decisions.md` — settled questions
