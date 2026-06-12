# Getting started with Shale

From zero to a PR card in about five minutes. No account, no server — just a
binary, your repo, and your existing CI.

---

## How the pieces connect

Before the numbered steps, here is what Shale actually does end to end:

```
LAPTOP (gitignored)                         GITHUB / CI
─────────────────────────────────────────   ──────────────────────────────────
Agent reads CLAUDE.md / AGENTS.md
  → sees steering block
  → calls: shale intent "Add rate limiting…"
           writes .shale/local/<session>.jsonl

Agent edits files
  → hooks stream each touch into .jsonl

Agent calls: shale done --note "…"
  → appends completion event
  → finalizes on the spot:
     reads .shale/local/<session>.jsonl
     redacts → folds → writes .shale/<session>.yaml
     commits the evidence file

git push
  → evidence commit rides along
  → pre-push hook runs shale finalize --auto-commit
    as a safety net for sessions without a done
                                              PR opened / updated
                                                → pull_request_target fires
                                                  (runs workflow from default branch)

                                              shale render
                                                → fetches PR diff via API
                                                → fetches .shale/*.yaml via API
                                                  (NO code checkout — ever)
                                                → renders card markdown
                                                → posts comment to PR
                                                → posts neutral check run
```

Key points:
- **Everything on the left stays on the laptop** — events are in gitignored
  `.shale/local/`; raw prompts are never included in committed evidence.
- **The evidence commit rides with your push** — `shale done` finalizes and
  commits on the spot, so the commit exists before you push and lands with the
  code. The pre-push hook is the safety net for sessions that never called
  `done`.
- **The renderer never touches your code** — it reads the diff and `.shale/`
  files through the GitHub API. No checkout means no privilege escalation on
  `pull_request_target`.

---

## 1. Install the CLI

**Homebrew (macOS / Linux):**

```sh
brew install provasign/shale/shale
```

Newer Homebrew versions ask you to trust third-party taps once — if brew
refuses with "untrusted tap", run `brew trust provasign/shale` and retry.
The same applies to `brew upgrade` later.

**Direct download:** grab the archive for your platform from the
[latest release](https://github.com/provasign/shale/releases/latest), verify
the checksum if you like (each asset has a `.sha256`), and put `shale` on your
`PATH`:

```sh
tar -xzf shale_$(uname -s | tr A-Z a-z)_arm64.tar.gz
sudo install -m 0755 shale /usr/local/bin/shale
```

Confirm:

```sh
shale version
```

---

## 2. Initialize your repo

```sh
cd your-repo
shale init
```

You'll see something like:

```text
  ✓ Steering prompt added      (CLAUDE.md, AGENTS.md)
  ✓ Wrote repo capture hooks   (.claude/settings.json, .github/hooks/shale.json,
                                .cursor/hooks.json, .codex/hooks.json)
  ? Auto-approve shale evidence commands (shale intent/done/note) for Claude Code?
    Writes a permissions allowlist into the committed .claude/settings.json so your
    team's agents stop prompting for exactly these three commands. [y/N] y
  ✓ Claude auto-approve        (shale intent/done/note allowlisted in .claude/settings.json)
  ✓ Created .shale/            (committed; .shale/local/ gitignored)
  ✓ Wrote .github/workflows/shale.yml
  ✓ Installed pre-push hook    (runs shale finalize)
  → Commit these files. Contributors and their agents are wired on clone.

The `[y/N]` question only appears at a real terminal (never in scripts or
CI — pass `--allow-agent-commands` there). Answering `y` writes a Claude
Code permissions allowlist scoped to exactly `shale intent`, `shale done`,
and `shale note` into the committed settings, so nobody on the team gets
permission prompts for the evidence commands. `shale uninstall --repo`
removes it cleanly.
```

What each piece does:

| What | Why |
|---|---|
| Steering prompt (`CLAUDE.md`, `AGENTS.md`, …) | Tells your agent to declare `shale intent` before editing and `shale done` after — the semantic evidence tier, works with *any* agent |
| Repo capture hooks | Stream file touches, commands, and prompts into the session record for agents with hook support. **Self-guarding**: teammates without Shale installed see zero errors |
| `.shale/` | Committed, schema-versioned, redacted evidence. `local/` working state is gitignored |
| `.github/workflows/shale.yml` | Renders the card on every PR via the GitHub API — never checks out PR code |
| Pre-push hook | Safety net: finalizes any session `shale done` didn't (interrupted agents, sessions without a done) and commits the evidence. Fail-open: it can never block your push |

### The card-rendering workflow

The workflow `shale init` writes runs on `pull_request_target` — which means
it **always runs from the default branch**, not from the PR branch. This is
intentional: it gives the renderer a write-capable `GITHUB_TOKEN` that works
even for fork PRs, without the privilege-escalation risk of checking out
untrusted PR code.

What this means for you:

> **The workflow must be merged to your default branch before any PR will get
> a card.** Until that merge happens, the workflow simply doesn't exist from
> GitHub's perspective.

The workflow requests exactly the permissions it needs — no repository-wide
secrets are required:

```yaml
permissions:
  contents: read        # fetch .shale/*.yaml files via API
  pull-requests: write  # post the card comment
  checks: write         # post the neutral check run
```

These explicit permissions work even if your organisation's default is
`GITHUB_TOKEN: read-only`.

### Repos with branch protection

If your default branch (`main`, `master`) requires pull requests before
merging, follow this bootstrap sequence:

```sh
# 1. Create a branch for the Shale setup commit
git checkout -b chore/enable-shale

# 2. Run init and commit
shale init
git add .
git commit -m "chore: enable shale"

# 3. Push and open a PR
git push -u origin chore/enable-shale
# → open a PR against main in the GitHub UI
```

Open the PR normally and get it reviewed. **This first PR will not have a
Shale card** — that's expected, because the workflow doesn't exist on `main`
yet. That is the bootstrap PR.

Once it merges, every subsequent PR will get a card. There is no second
bootstrap step.

> **If your org requires Actions approval for new workflows:** an org admin
> needs to approve the `shale.yml` workflow once after it lands on `main`.
> This is a one-time step done in your org's Actions settings.

### Repos that already use hooks or other PR bots

`shale init` is non-destructive on an existing repo: scaffold files that
already exist are skipped (your `.shale/config.yaml` privacy choice is never
overwritten), steering blocks are appended to existing instruction files, and
agent hook configs are merged additively.

**Other PR comment bots are fine.** Shale finds its own comment by a hidden
marker and edits only that one — it never touches comments posted by other
tools. Their card and Shale's card coexist as separate comments.

**Existing pre-push hooks are the one thing to check.** Shale never clobbers
a hook it didn't write. `shale init` honors `core.hooksPath` (husky,
lefthook), so the hook lands where git will actually run it — but if a hook
manager already owns the `pre-push` file, init prints
`! Pre-push hook skipped` and you chain Shale in yourself:

```sh
shale finalize --auto-commit || echo "shale: finalize failed (push continues)" >&2
```

Add that line to `.husky/pre-push` (husky), a `pre-push` job in
`lefthook.yml` (lefthook), or your existing `.git/hooks/pre-push`. Without
it, finalize never runs on push and PRs get the "No shale for this PR" nudge
instead of a card. `shale doctor` reports the exact hook location it checks.

### Options

```sh
shale init --privacy full|redacted|hash-only   # prompt persistence (default: redacted)
shale init --global                            # also wire ~/.claude, ~/.cursor, … machine-wide
shale init --hooks-only                        # just the pre-push hook (fork contributors)
```

---

## 3. Run an agent session

Work with your agent as usual. Behind the scenes:

1. The steering prompt makes the agent declare its goal **before editing**:
   ```
   shale intent "Add rate limiting to the login endpoint" --body "Token bucket, 10 req/min…"
   ```
2. Hooks (where the agent has them) record every file touch, command, and
   prompt into gitignored `.shale/local/` — laptop-only working state.
3. When the work is done, the agent reports:
   ```
   shale done --note "…" --model … --tokens-in … --tokens-out …
   ```
4. `git push` triggers the pre-push hook → `shale finalize --auto-commit`
   folds the session into `.shale/<session>.yaml`, commits it, and lets the
   push through. Raw prompt text is **not** included — see
   [privacy](#what-the-evidence-looks-like) below.

> **Note:** the finalize commit is created during the push. If you open a PR
> immediately after the first push, the evidence is already there. If you push
> and then amend or force-push, finalize is idempotent — push once more and
> the evidence lands correctly.

---

## 4. Open a PR and read the card

The workflow renders a card like
[this live example](https://github.com/provasign/shale-test-bed/pull/1):
intent, completion note, model/token/cost summary, a per-file evidence table,
locally recorded checks, and any warnings (sensitive paths, files with no
session evidence, tamper flags).

The card is **advisory**: the check is always neutral/success, never failing.
A PR with no evidence at all gets an explicit "No shale for this PR" nudge —
absence is visible, not silent.

**Not using GitHub Actions?** See [CI integrations](ci-integrations.md) for
Jenkins, CircleCI, GitLab CI, and generic pipelines.

---

## 5. Verify your setup anytime

```sh
shale doctor
```

One line per check, with the exact fix when something's wrong. To preview a
card locally without opening a PR:

```sh
shale render --local
```

---

## What the evidence looks like

```text
.shale/
  config.yaml                  # privacy mode (committed)
  25f37dfc-….yaml              # one file per agent session
  local/                       # gitignored — raw events, prompts; never committed
```

Session files record intent, files touched (with hook/git provenance),
commands with exit codes, model/token/cost metadata, and timestamps —
agent-authored text passes a secret-redaction pass first. Editing evidence
after capture puts a tamper warning on the card.

**Never recorded:** file contents or diffs (only the path and operation),
gitignored files (`.env`, `*.pem`, `secrets/` — dropped at finalize, since
that's where secrets live), and files outside the repo root. **Masked before
persistence:** secrets in intent/notes/commands — vendor token shapes plus
command-line forms (`API_KEY=… ./run`, `--token=…`, `--password …`, bearer
headers). Commands themselves stay on the card; only the secret values are
masked.

**Raw prompts never leave your laptop.** Shale can capture and redact the
prompts developers type, but committing prompt transcripts is **disabled in
the product** (a build-time flag, not configurable) until the redaction
layer is proven against free-form human text and the regulatory questions
around persisting raw prompts are settled. Committed evidence carries only a
prompt count and an intent integrity hash.

---

## Uninstalling

```sh
shale uninstall          # this machine: pre-push hook, global agent hooks, .shale/local/
shale uninstall --repo   # …plus the committed files — commit the removal for your team
brew uninstall shale     # the binary
```

Only what Shale wrote is removed: your instruction-file content survives
(the fenced block is cut), foreign hook entries are preserved, and a hook
manager's pre-push file is never edited (remove the chained
`shale finalize` line yourself). Shortcut: `brew uninstall shale` alone
makes every committed hook silently inert for you — that's the self-guarding
design working in reverse.

---

## Next steps

- [CI integrations](ci-integrations.md) — Jenkins, CircleCI, GitLab CI, GitHub Enterprise Server
- [Troubleshooting](troubleshooting.md) — hooks not firing, card not updating, Windows notes
- [Product, architecture & design](product.md) — how capture works per agent, the card, and the design decisions
- [Shale file spec](shale-spec.md) — the evidence format, for tool builders
