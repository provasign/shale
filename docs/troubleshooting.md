# Troubleshooting Shale

Start with the doctor — it covers most of this page automatically:

```sh
shale doctor
```

Every failing check prints the exact fix. What follows is the longer story for
each failure mode, in the order people usually hit them.

## Install

### `shale: command not found` after brew install

Make sure the tap-qualified name was used and your shell has rehashed:

```sh
brew install provasign/shale/shale
hash -r        # or open a new terminal
shale version
```

### Hooks don't fire from GUI-launched editors (macOS)

Apps launched from the Dock/Finder don't inherit your shell `PATH`. The
repo-level hook commands Shale writes are guarded
(`command -v shale … && shale capture …`), so a missing PATH entry is a
**silent no-op**, not an error — but it also means no capture. Fixes:

- launch the editor from a terminal (`cursor .`, `code .`), or
- install Shale somewhere GUI apps see (`/usr/local/bin` or `/opt/homebrew/bin`
  are fine on modern macOS), then restart the editor.

## Capture

### The card shows `◐ … not hook-verified` for every file

The session had intent/done but no hook events, so the file list came from git
(the fallback tier). That's expected for agents without a hook adapter yet
(Cursor, Codex, Copilot — adapters in progress). For Claude Code it usually
means the hooks aren't firing:

1. `shale doctor` — is "capture hooks" green?
2. Check the repo has `.claude/settings.json` with `shale capture claude-code`
   entries (or `~/.claude/settings.json` from `shale init --global`).
3. Restart Claude Code — hook config is read at startup.

### No intent on the card / "No intent declared"

The agent never ran `shale intent`. Check the steering block survived in your
agent's instruction file (`CLAUDE.md`, `AGENTS.md`, `.cursorrules`, …) — it's
fenced between `<!-- shale-start -->` and `<!-- shale-end -->`. Agents follow
it reliably once it's in context; if your agent uses a file Shale doesn't
write by default, copy the block there.

### Capture problems leave no trace

By design — capture must never disturb the agent. Errors go to
`.shale/local/capture.log`, so look there first when events seem to vanish.

## Finalize and push

### Evidence isn't on the PR even though the session ran

The pre-push hook creates the evidence commit **during** the push, so that
commit stays local until the next push:

```sh
git push        # once more — the evidence commit goes up
```

If you amended or rebased instead, just push again; finalize is idempotent.

### Pre-push hook didn't install

An existing non-Shale `pre-push` hook is never clobbered. Add this line to
your hook yourself:

```sh
shale finalize --auto-commit || echo "shale: finalize failed (push continues)" >&2
```

### `finalize` skipped a session

Already-finalized sessions are archived and skipped — that's idempotency, not
data loss. Active session events live in `.shale/local/`, archives in
`.shale/local/archive/`.

## The card

### No card appears on the PR

1. The workflow must exist on the **default branch** — `pull_request_target`
   runs the workflow from there, not from the PR branch. Merge the
   `shale init` commit first.
2. Check the run: repo → Actions → `shale`. A 404 downloading the binary
   means the pinned version was yanked — bump `version:` in the workflow or
   take the action default.
3. The workflow needs `pull-requests: write` and `checks: write` permissions
   (the `shale init` template sets these).

### The card didn't update after I changed something

Cards re-render when the workflow runs — on PR open/synchronize/reopen. A new
Shale release doesn't rewrite existing cards until something triggers the
workflow again (push a commit, or re-run the workflow from the Actions tab).

### Warnings about `.shale/` files or `config.yaml` on the card

Fixed in v0.1.1+. Update the action's `version:` input (or use the `v1` tag,
which tracks the latest release).

### "transcript hash mismatch" / "edited after capture" warnings

> Current builds don't write transcripts at all (raw prompt capture is
> feature-flagged off — see the README's Privacy section). Hash-mismatch
> warnings can still appear for evidence committed by older versions.

Working as intended: someone modified evidence after it was finalized. The
card flags it and treats the affected content as unverified. If this was a
legitimate rebase artifact, re-running the session or removing and
re-finalizing the evidence clears it.

## Security posture (things that look like bugs but aren't)

- **The check never fails.** Shale is advisory by design; it will not block a
  merge, ever. Don't wire branch protection to it expecting a gate.
- **The workflow never checks out PR code.** It reads everything via the API.
  Adding `actions/checkout` to `shale.yml` reintroduces a classic
  `pull_request_target` privilege escalation — `shale doctor` flags it.
- **Hook config in the repo is executable config.** Some agents ask for trust
  before running repo-committed hooks (Codex: one-time `/hooks` review;
  Cursor/VS Code: workspace trust). That prompt is the agent doing its job.

## Windows

- The repo-level hook guards are POSIX (`command -v`) and run fine under
  Claude Code's Git Bash. Codex entries carry a `commandWindows` variant.
- Building from source or running tests: golden files require LF — the repo's
  `.gitattributes` handles it, but check `core.autocrlf` if you see diff noise.

## Still stuck?

Open an issue with the output of `shale doctor` and `shale version`:
<https://github.com/provasign/shale/issues>. Nothing in either output contains
prompt content.
