# Getting started with Shale

From zero to a PR card in about five minutes. No account, no server — just a
binary, your repo, and your existing CI.

## 1. Install the CLI

**Homebrew (macOS / Linux):**

```sh
brew install provasign/shale/shale
```

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
  ✓ Created .shale/            (committed; .shale/local/ gitignored)
  ✓ Wrote .github/workflows/shale.yml
  ✓ Installed pre-push hook    (runs shale finalize)
  → Commit these files. Contributors and their agents are wired on clone.
```

What each piece does:

| What | Why |
|---|---|
| Steering prompt (`CLAUDE.md`, `AGENTS.md`, …) | Tells your agent to declare `shale intent` before editing and `shale done` after — the semantic evidence tier, works with *any* agent |
| Repo capture hooks | Stream file touches, commands, and prompts into the session record for agents with hook support. **Self-guarding**: teammates without Shale installed see zero errors |
| `.shale/` | Committed, schema-versioned, redacted evidence. `local/` working state is gitignored |
| Workflow file | Renders the card on every PR via API — it never checks out PR code |
| Pre-push hook | Finalizes any open session and commits the evidence before push. Fail-open: it can never block your push |

Commit and push:

```sh
git add . && git commit -m "chore: enable shale" && git push
```

Done. One person runs `init`; everyone else gets the wiring on `git clone`.

### Options

```sh
shale init --privacy full|redacted|hash-only   # prompt persistence (default: redacted)
shale init --global                            # also wire ~/.claude, ~/.cursor, … machine-wide
shale init --hooks-only                        # just the pre-push hook (fork contributors)
```

## 3. Run an agent session

Work with your agent as usual. Behind the scenes:

1. The steering prompt makes the agent declare its goal **before editing**:
   `shale intent "Add rate limiting to the login endpoint" --body "Token bucket, 10 req/min…"`
2. Hooks (where the agent has them) record every file touch, command, and
   prompt into `.shale/local/` — redacted as configured.
3. When the work is done, the agent reports:
   `shale done --note "…" --model … --tokens-in … --tokens-out …`
4. `git push` triggers the pre-push hook → `shale finalize --auto-commit`
   folds the session into `.shale/<session>.yaml` plus a prompts-only
   transcript, commits them, and lets the push through.

> **Note:** the finalize commit is created during the push, so it stays local
> until the *next* push. Push once more (or amend your workflow to push after
> finalize) and the evidence lands with the PR.

## 4. Open a PR and read the card

The workflow renders a card like
[this live example](https://github.com/provasign/shale-test-bed/pull/1):
intent, completion note, model/token/cost summary, a per-file evidence table,
locally recorded checks, and any warnings (sensitive paths, files with no
session evidence, tamper flags).

The card is **advisory**: the check is always neutral/success, never failing.
A PR with no evidence at all gets an explicit "No shale for this PR" nudge —
absence is visible, not silent.

## 5. Verify your setup anytime

```sh
shale doctor
```

One line per check, with the exact fix when something's wrong. To preview a
card locally without opening a PR:

```sh
shale render --local
```

## What the evidence looks like

```text
.shale/
  config.yaml                  # privacy mode (committed)
  25f37dfc-….yaml              # one file per agent session
  transcripts/25f37dfc-….md    # prompts-only, redacted, SHA-256 pinned
```

Session files record intent, files touched (with hook/git provenance),
commands with exit codes, model/token/cost metadata, and timestamps. The
transcript hash is verified at render time — editing evidence after the fact
puts a tamper warning on the card.

## Next steps

- [Troubleshooting](troubleshooting.md) — hooks not firing, card not updating, Windows notes
- [Product, architecture & design](product.md) — how capture works per agent, the card, and the design decisions behind them
- [Shale file spec](shale-spec.md) — the evidence format, for tool builders
