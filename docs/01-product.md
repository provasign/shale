# Shale — Product

## 1. Positioning

| Question | Answer |
|---|---|
| Category | Agent PR evidence / provenance layer (cross-agent, local-first) |
| Primary buyer-user | The PR reviewer / maintainer drowning in agent-generated PRs |
| Wedge | Intent capture + intent↔diff conformance rendered on the PR |
| What we refuse to be | A gate runner (Sonar/Semgrep MCP own in-loop checks), an LLM reviewer (CodeRabbit/Greptile own that), a GitHub-only feature (GitHub owns Copilot tracing) |
| Default posture | Advisory, fail-open, zero accounts, zero servers |
| CI surface | GitHub Action for OSS reach; the same binary runs in **any CI** (Jenkins, GitLab CI, CircleCI) via a documented env contract — enterprises rarely run Actions |
| License | Apache-2.0 |

**Market gap this occupies** (validated 2026-06): GitHub traces commits to
session logs *only* for its own Copilot cloud agent. Git AI does line-level
attribution but no intent card and no check evidence. SpecStory saves session
transcripts but renders nothing for reviewers. Nobody combines
intent + diff conformance + recorded checks on the PR surface, cross-agent.

## 2. Personas and what each needs from the card

| Persona | Their question | Card answer |
|---|---|---|
| **PR reviewer** (primary) | "What was the agent asked, and does this diff match the ask?" | Intent block (verbatim prompt), changed-files table, in-scope/out-of-scope flags (MVP 3), session links |
| **Author** | "I want trust without ceremony" | Everything automatic after `shale init`; nothing to remember |
| **Security engineer** | "Did anything sensitive change? Did secrets/SAST run?" | Sensitive-path flags (auth, crypto, CI config, deps manifests), recorded check results with timestamps |
| **Risk / compliance** | "Show me an audit trail for AI-written code" (EU AI Act, SOC 2) | Shale YAML is a durable, committed, schema-versioned record: prompt, model, agent, timestamps, transcript hash |
| **Eng manager / product owner** | "Which changes were agent-driven and against which intent?" | Intent titles across PRs; agent/model/session stats |

One card serves all five in MVP 1 (single layout, clearly sectioned). Persona
emphasis (collapsible sections, security-first ordering on sensitive-path PRs)
is MVP 2.

## 3. User experience

### 3.1 Setup — the 5-minute promise (this flow is the product; protect it)

```
$ brew tap provasign/shale
$ brew install shale
$ cd my-repo
$ shale init
  ✓ Steering prompt added      (CLAUDE.md, AGENTS.md, .cursorrules,
                                .github/copilot-instructions.md — tells the
                                agent when to call shale intent / shale done)
  ✓ Wrote repo capture hooks   (.claude/settings.json, .github/hooks/shale.json,
                                .cursor/hooks.json, .codex/hooks.json
                                — all agents, inert without shale on PATH)
  ✓ Created .shale/            (committed; .shale/local/ gitignored)
  ✓ Wrote .github/workflows/shale.yml   (renders the card on PRs)
  ✓ Installed pre-push hook    (runs shale finalize)
  → Commit these files and open your next PR as usual. Done.
```

The steering prompt is the universal layer — it works with **every** agent
that reads an instruction file (Claude Code, Cursor, Codex, Copilot, Gemini,
whatever ships next), because `shale intent` / `shale done` are plain CLI
calls (ADR D4). Hooks, where the agent has them, add verified file-level
evidence on top. Repo-level hook config is committed by default and guarded:
if a contributor has not installed `shale`, the hook command silently no-ops.
Agents with trust gates, such as Codex, activate the hook after their normal
repo-hook review flow.

Hard constraints on this flow:
- **≤ 2 commands** (`install`, `init`). Everything else is a question with a default.
- **No account, no token paste, no server URL.** The Action uses the repo's own
  `GITHUB_TOKEN`.
- `shale init` must be **idempotent** and must **never overwrite** user
  content without showing a diff and asking.
- `shale doctor` diagnoses a broken setup in one command.

### 3.2 Daily flow — zero behavior change for the developer

```
dev prompts agent
  → agent calls `shale intent "Add rate limiting to login endpoint"
                 --body "Brute force in prod. Redis, 10/min, 429+Retry-After. Tests required."`
     (steered by CLAUDE.md block written by shale init)
  → hooks capture file-touches + commands silently throughout session
  → agent calls `shale done --note "Redis limiter done. Fallback added. 3 files, 12 tests."
                             --tokens-in 32000 --tokens-out 15000 --model claude-fable-5`
agent edits and commits as usual
git push → pre-push hook runs `shale finalize` (mechanical: JSONL → YAML, redact, commit)
PR opened → Action renders the Shale card (comment + neutral Check Run)
```

The **developer** does nothing differently — they prompt the agent as usual.
The **agent** calls `shale intent` and `shale done` automatically by following
the steering prompt. `shale note "..."` remains as a manual escape hatch.

### 3.3 The Shale card (MVP 1 target rendering)

Posted as a PR comment (updated in place on synchronize) **and** a neutral
Check Run named `shale`.

```markdown
## 🧾 Shale · 2 sessions · Claude Code (claude-fable-5)
claude-fable-5 · 47k tokens · ~$0.47 · 3 iterations · 39 min

### Intent
> **Add rate limiting to the login endpoint**
>
> Brute force attempts observed in prod logs. Redis counter, 10 req/min per IP,
> return 429 with Retry-After header. Tests required.

*Declared 2026-06-09 14:02 · session `a1b2c3` · 14 prompts ·
transcript hash `sha256:9f1c…`*

### Completion
> Redis-backed rate limiter implemented. In-memory fallback added when Redis
> unavailable. 3 files changed, 12 tests added.

### Changed files (8) — 6 seen in agent sessions, 2 not
| File | Agent session | Notes |
|---|---|---|
| internal/auth/ratelimit.go      | ✅ a1b2c3 | new file |
| internal/auth/ratelimit_test.go | ✅ a1b2c3 | |
| internal/auth/login.go          | ✅ a1b2c3 | |
| go.mod                          | ⚠️ none   | dependency manifest changed |
| .github/workflows/deploy.yml    | ⚠️ none   | **sensitive path: CI config** |
| …                               |           | |

### Checks recorded locally
| Check | Result | When |
|---|---|---|
| gitleaks (agent-invoked)        | ✅ 0 findings | 14:31 |
| go test ./internal/auth/...     | ✅ 12 passed  | 14:33 |

*Recorded from the agent session — advisory only. CI remains authoritative.*

### Coverage gaps
⚠️ 2 changed files have no session evidence. They may be hand-edits or
changes from an uninstrumented tool.
```

Additional card behaviors:
- **Evidence provenance:** file rows distinguish hook-verified touches
  (✅ session id) from git-derived ones (◐ "changed during session a1b2c3 —
  not hook-verified", used for agents without hook adapters, e.g. Copilot).
  Honest labels, never blended (spec rule 9).
- **Large PRs:** the file table caps at 20 rows; beyond that, files group by
  top-level directory with counts, full list in a collapsible `<details>`
  block. Sensitive-path and no-evidence files always stay above the fold.
- **Tamper flags:** if a committed transcript no longer matches its recorded
  hash, or a shale file was hand-edited after capture (visible in the PR's
  own commit history), the card says so:
  `⚠️ shale edited after capture — treat intent text as unverified`.

Design rules:
- **Absence is explicit, never silent.** "No shale for this PR" is a card too.
- **Never block.** Check Run conclusion is `neutral` (or `success`); strict mode
  is a documented future opt-in, not a v1 toggle.
- The card is plain GitHub-flavored markdown — no images, no external assets,
  renders identically in GitHub/Gitea mirrors of the comment.

### 3.4 The no-shale / partial flow (most PRs at first — make it great)

A contributor without Shale opens a PR on a repo that has the Action:

```markdown
## 🧾 No shale for this PR
This repo renders agent evidence on PRs. No agent session evidence was found
for these commits. If you used an AI agent: `brew tap provasign/shale && brew install shale && shale init`
(5 minutes, no account). If this was hand-written, ignore this — humans don't
need shale. 🙂
```

One comment per PR, never repeated, never failing the build. This message is
the growth loop — every uninstrumented PR advertises the tool to exactly the
right person at the right moment.

### 3.5 Reviewer flow

The reviewer reads the card *before* the diff: intent first, then the
out-of-scope warnings, then recorded checks. Links jump to the transcript file
in `.shale/` at the PR's head SHA. Reviewing the shale is reviewing the PR
description — except it's generated, trustworthy-by-construction (hash-bound to
the session), and consistent across every agent PR in the org.

### 3.6 External contributor on a fork

1. Contributor forks the repo — the fork already contains `.shale/`
   scaffold and the workflow file (they're committed upstream).
2. The fork also carries Shale's repo-level hook config. If the contributor has
   `shale` on `PATH` and their agent trusts repo hooks, capture starts
   automatically; otherwise the hook is a silent no-op and the steering + git
   fallback still produce honest evidence. They may run `shale init --global`
   for machine-wide capture, but there is **no upstream registration, no token,
   no "connect" step.**
3. They work, push to their fork, open the PR — shale files ride along as
   committed files and the upstream card renders with full evidence. (Token
   mechanics: the workflow uses `pull_request_target` and never checks out PR
   code — see architecture §3.7.)
4. If they don't use Shale, they get the standard no-shale nudge —
   which is exactly the audience that comment exists for.

Same UX for fork and same-repo PRs. This zero-config fork story is a core
differentiator versus server-registration models.

### 3.7 Enterprise CI (Jenkins / GitLab) — no Actions required

`shale render` is a plain binary with an env contract (architecture §3.7):
drop one stage into a `Jenkinsfile` or `.gitlab-ci.yml` and the same card
appears as a PR comment (GitHub via Jenkins plugins) or MR note (GitLab driver,
MVP 2). The GitHub Action is a convenience wrapper for OSS, not a requirement —
this matters because most enterprises run Jenkins or GitLab CI, not Actions.

## 4. What ships when (summary — full plan in 04-implementation-plan.md)

| Milestone | Headline | Proves |
|---|---|---|
| **MVP 1** | `shale intent`/`shale done` via steering prompt (**every agent**, incl. Copilot) + Claude Code hook capture + git-fallback file evidence + `.shale/` + Action card | The 5-minute promise, the growth loop, universal coverage day one |
| **MVP 2** | Cursor/Codex/Gemini hook adapters (verified file evidence), GitLab driver + Jenkins recipe, recorded checks, spec v1 published, `shale verify` | Cross-agent **and cross-CI** depth (the moat vs GitHub) |
| **MVP 3** | Grove-backed intent↔diff conformance, sensitive-path policy file, git-notes storage mode, `shale gc`, Provasign bridge | The killer feature; the enterprise on-ramp |

## 5. Success metrics

- **Activation:** stranger → card on a real PR in ≤ 5 min (measured by docs
  walkthrough timing; this is the only metric that matters for MVP 1).
- **Coverage:** % of changed files in a PR with session evidence (the product
  improves as this rises; surfaced on the card itself).
- **Growth loop:** installs attributable to the no-shale comment.
- **Retention proxy:** repos where the Action runs on ≥ 10 PRs.

## 6. Non-goals (v1–v3)

- Running scanners/tests itself (record only)
- LLM-generated review commentary
- Line-level authorship attribution (Git AI does this; we link at file/session granularity)
- Blocking merges, branch protection, server-side policy
- Any hosted backend (that's Provasign's tier)
