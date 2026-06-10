# Shale — Agent hook mechanisms & the activation model

How Shale's mechanical tier (ADR D4 tier 2) wires into each agent, where each
agent reads hook config, and how a single global install is gated repo-by-repo.
This doc backs the MVP 2 adapter work (implementation plan F1–F3) and the
`shale init` redesign below.

Status: §3/§4 (repo-level init, steering, doctor, `--global`) are **implemented**
(2026-06-10). Capture adapters beyond Claude Code remain MVP 2 — their committed
hook configs are written today and are inert by design until each adapter ships
(`shale capture` fails open on unknown adapters).

---

## 1. The two questions every adapter answers

A hook integration is two independent decisions:

1. **Capture** — given the agent's hook payload on stdin, parse it into
   `store.Event`s. This is the `internal/capture/<agent>.go` parser. Pure,
   total, fail-open (see `capture/claudecode.go`).
2. **Wiring** — write the agent's hook-config file so the agent runs
   `shale capture <agent>` on the right events. This is the `initx` half.

The capture pipeline (`capture_command.go` → `store` → `fold` → `render`) is
**agent-agnostic**. Adding an agent is one parser + one wiring function. Nothing
downstream changes.

---

## 2. Hook mechanisms by agent

Every popular coding agent now exposes a shell-command hook system with the same
basic shape: **the agent spawns a command and pipes a JSON event payload to its
stdin; the command may return JSON on stdout; exit code influences flow.** They
differ in config-file location, field names, event names — **and in coverage**:
which tool calls actually fire hooks varies per agent (see §2.2), so adapters
must be capability-driven, not just case-normalization.

All claims below were verified against official vendor docs on 2026-06-10:
[Claude Code hooks](https://code.claude.com/docs/en/hooks),
[Codex hooks](https://developers.openai.com/codex/hooks),
[Cursor hooks](https://cursor.com/docs/hooks),
[VS Code agent hooks](https://code.visualstudio.com/docs/agent-customization/hooks),
[GitHub Copilot hooks reference](https://docs.github.com/en/copilot/reference/hooks-reference).
Docs drift; **record fixtures from a real session before finalizing any parser.**

### 2.1 Capability matrix

| Agent | Global config | Repo / project config | Session-id field | CWD field | Trust gate for repo hooks | Maturity |
|---|---|---|---|---|---|---|
| **Claude Code** | `~/.claude/settings.json` | `.claude/settings.json` (checked in) · `.claude/settings.local.json` (gitignored) | `session_id` | `cwd` | **None documented** — project hooks run on clone (enterprise `allowManagedHooksOnly` can block) | GA. Implemented in Shale today. |
| **Codex CLI** | `~/.codex/hooks.json` · inline `[hooks]` in `~/.codex/config.toml` | `<repo>/.codex/hooks.json` · inline `[hooks]` in `<repo>/.codex/config.toml` | `session_id` | `cwd` | **Yes — explicit review required**: non-managed command hooks must be reviewed and trusted (`/hooks` command) before they run | Enabled by default (`codex_hooks` is a deprecated alias; disable via `[features] hooks = false`). Windows supported via `commandWindows`/`command_windows`. |
| **Cursor** | `~/.cursor/hooks.json` | `<project>/.cursor/hooks.json` ("checked into version control with your project") | `conversation_id` | `workspace_roots[0]` | Workspace trust — project hooks run in any **trusted workspace** | GA since v1.7. Payload carries `transcript_path`. |
| **VS Code Copilot** | `~/.copilot/hooks` · `~/.claude/settings.json` | `.github/hooks/*.json` · `.claude/settings.json` · `.claude/settings.local.json` | `sessionId` (`session_id` in Claude-format files) | `cwd` | None documented; org admins can disable hooks entirely | **Preview** — "configuration format and behavior might change". Locations controlled by `chat.hookFilesLocations`, which **by default enables `.claude/settings.json`, `.claude/settings.local.json`, `~/.claude/settings.json`, and `.github/hooks`**. |
| **Copilot CLI** | `~/.copilot/hooks/` (`%USERPROFILE%\.copilot\hooks\` on Windows) | `.github/hooks/*.json` in repo root · inline in repo `settings.json` | `sessionId` (camelCase) or `session_id` (VS Code-compat format) | `cwd` | None documented; policy dir (`/etc/github-copilot/policy.d/`) for admin control | GA. GitHub's doc scopes hooks to "Copilot CLI and Copilot cloud agent"; cloud agent reads `.github/hooks/*.json` only. |

Two findings worth calling out because they were disputed and then confirmed:

- **VS Code Copilot reading Claude-format hook files is officially documented**,
  not folklore: the default value of `chat.hookFilesLocations` lists
  `.claude/settings.json` (workspace and user) alongside `.github/hooks`. One
  committed `.claude/settings.json` therefore wires Claude Code *and* VS Code
  Copilot. The caveat is real, though: the VS Code feature is **Preview**, so
  this dual coverage must be fixture-tested per release, and the
  `.github/hooks/shale.json` file is the stable fallback for Copilot if the
  Claude-format bridge changes.
- **Repo-level hook config is confirmed for all five agents** from official
  docs (locations quoted in the table). What varies is the **trust UX**, which
  is the honest cost of repo-level wiring — see §2.3.

### 2.2 Event mapping — and per-agent coverage caveats

Shale needs four moments. Each agent names them differently.

| Shale needs → | `store` kind | Claude Code | Codex CLI | Cursor | VS Code Copilot / Copilot CLI |
|---|---|---|---|---|---|
| session opened (id, model) | `KindSessionMeta` | `SessionStart` | `SessionStart` | `sessionStart` | `SessionStart` / `sessionStart` |
| user prompt (transcript) | `KindPrompt` | `UserPromptSubmit` | `UserPromptSubmit` | `beforeSubmitPrompt` | `UserPromptSubmit` / `userPromptSubmitted` |
| file/command touch | `KindFileTouch` / `KindCommand` | `PostToolUse` | `PostToolUse` (see caveats) | `postToolUse` + `afterFileEdit` | `PostToolUse` / `postToolUse` |
| session ended | `KindSessionEnd` | **`SessionEnd`** (see note) | `Stop` | `sessionEnd` | `SessionEnd` / `sessionEnd` |

**Stop vs SessionEnd (Claude Code).** These are different events: `Stop` fires
**every turn** when Claude finishes responding (blockable, repeats); `SessionEnd`
fires **once** at session termination (non-blockable). Shale's current wiring
maps `Stop` → `KindSessionEnd`, which is harmless in practice (finalize is
idempotent and the real boundary is the pre-push fold) but semantically wrong.
The adapter should wire **both**: `SessionEnd` → `KindSessionEnd`, and keep
`Stop` as a cheap last-activity marker. Copilot mirrors this distinction
(`agentStop` per-turn vs `sessionEnd`).

**Codex coverage caveats (why parsers must be capability-driven):**

- `PreToolUse`/`PostToolUse` "doesn't intercept all shell calls yet, only the
  simple ones", and does **not** intercept non-shell, non-MCP tools.
- File edits surface as `tool_name: "apply_patch"` (matchers accept
  `apply_patch`, `Edit`, or `Write` as aliases) — the Codex parser must map
  `apply_patch` payloads to `KindFileTouch`, not expect Claude-style
  `Edit`/`Write` tool names.
- Practical consequence: Codex hook evidence will be **partial** even when
  wired. That is fine — partial hook evidence merges with the git fallback at
  finalize (hook beats git per path, `render.writeFiles` already implements
  this precedence), and untouched-by-hook files simply render as `◐ via git`.

A shared `capture` helper should normalize PascalCase↔camelCase event names and
`session_id`↔`sessionId`↔`conversation_id`, but each parser additionally
declares what it *can't* see, so finalize knows to lean on the git fallback.

### 2.3 Global vs repo — and the trust UX that comes with repo-level

Confirmed from official docs:

> **Every agent supports repo-level hook config, not just global.**

- Claude Code: `.claude/settings.json` (checked in; documented as shareable)
- Codex CLI: `<repo>/.codex/hooks.json` or `[hooks]` in `<repo>/.codex/config.toml`
- Cursor: `<project>/.cursor/hooks.json`
- Copilot (CLI + cloud agent + VS Code): `.github/hooks/*.json`

A repo-level hook config:

- **travels with the repo** — every contributor (and the agent) gets it on
  `git clone`; no per-developer `shale init`;
- **is self-gating** — it only exists in repos that opted in, so there is no
  "fires everywhere" problem to solve;
- **is reviewable** — it lands in the PR like any other config;
- **leaves no machine state** — nothing in `~` to drift or clean up.

The honest costs — repo-committed hooks are **executable configuration**, and
the vendors treat them accordingly:

| Agent | What happens on clone, before any consent |
|---|---|
| Claude Code | Hooks **run** (no trust prompt documented) |
| Copilot CLI / cloud agent | Hooks **run** (no trust prompt documented) |
| VS Code Copilot | Hooks run in a trusted workspace (VS Code workspace trust) |
| Cursor | Hooks run only in a **trusted workspace** |
| Codex CLI | Hooks **do not run** until the user reviews and trusts them via `/hooks` |

So "wired on clone" is literally true for Claude Code and Copilot, true-after-
workspace-trust for Cursor and VS Code (a gate most developers have already
passed for any repo they work in), and **requires a one-time explicit approval
for Codex**. The design must not paper over this: for Codex, repo-level wiring
means "offered on clone, active after a one-keystroke review" — which is still
far less friction than a per-developer `shale init`, and arguably the right
security posture for committed executable config. Plus: each contributor needs
`shale` on `PATH` (§4.6 closes that gap).

---

## 3. The activation model (answering "global setup, activate per repo")

There are two activation gates in Shale, one per tier. Both are **already
repo-level today** — this is the existing design, made explicit.

### 3.1 Semantic tier — gated by the steering prompt (already repo-level)

`shale intent` / `shale done` only get called because an instruction file in the
repo tells the agent to call them. Those files — `CLAUDE.md`, `AGENTS.md`,
`.cursorrules`, `.github/copilot-instructions.md`, … — **live in the repo**
(`initx/steering.go`). No steering block in the repo ⇒ the agent never declares
intent ⇒ no semantic evidence. The repo *is* the switch.

So the user's intuition is exactly right: **the steering prompt is already the
per-repo activation for the universal tier.** A global agent install does
nothing in a repo until that repo carries the steering block.

### 3.2 Mechanical tier — gated by `.shale/` presence (already repo-level)

Today the Claude Code hook is installed **globally** (`~/.claude/settings.json`)
so it fires for *every* Claude Code session on the machine. It does nothing
unless the repo opted in. From `capture_command.go`:

```go
// Only capture into repos that opted in (have a .shale/ scaffold).
if _, err := os.Stat(filepath.Join(root, ".shale")); err != nil {
    return 0   // fail-open: no .shale/ dir → capture is a no-op
}
```

So even with a global hook, **activation is the presence of `.shale/` in the
repo**. Clone a repo without `.shale/`, the global hook fires and immediately
no-ops. Run `shale init` in a repo (creating `.shale/`), and the same global
hook starts capturing. The global install is already self-gating per repo.

### 3.3 The upgrade: make the mechanical tier repo-level too

Given §2.3 (every agent supports repo-level hook config), we can drop the global
install entirely and put the hook config **in the repo**, next to the steering
prompt and the `.shale/` scaffold:

```
repo/
  .shale/                      # opt-in marker + evidence (existing)
  .claude/settings.json        # hook → shale capture claude-code  (also drives VS Code Copilot)
  .github/hooks/shale.json     # hook → shale capture copilot       (VS Code Copilot + Copilot CLI)
  .cursor/hooks.json           # hook → shale capture cursor
  CLAUDE.md / AGENTS.md / …    # steering prompt (existing)
  .github/workflows/shale.yml  # card renderer (existing)
```

Now **one person runs `shale init` once, commits, and the team's agents are
wired on clone** — immediately for Claude Code and Copilot, after workspace
trust for Cursor/VS Code, and after a one-keystroke hook review for Codex
(§2.3 trust table). No machine-global config, no per-developer setup — the
config's presence in the repo is the activation, identical in spirit to how
steering already works. The `.shale/` existence check stays as a
belt-and-suspenders no-op guard.

Repo-level is the better default for teams; global (`--global`, §4.2) remains a
valid low-friction choice for individuals — particularly for Codex, where a
developer's own global hooks skip the per-repo trust review.

### 3.4 Hooks are best-attempt; steering + CLI is the floor

This is the load-bearing posture, inherited from ADR D4 and restated here
because the hook matrix above must never become a dependency:

**Shale must produce a useful card with zero hooks firing.** Every hook
integration is an *upgrade attempt*, and every failure mode — agent has no hook
system, hook config untrusted, hook API drifted, binary missing, Codex's partial
tool interception — degrades to the same well-defined floor:

| Evidence | With hooks | Without hooks (floor) |
|---|---|---|
| Intent (title/body) | `shale intent` (steered) | same — unchanged |
| Completion note, model, tokens, iterations | `shale done --model … --tokens-in …` (steered) | same — unchanged |
| Finalized YAML + tamper-evident hashes | yes | same — unchanged |
| File evidence | `via: hook`, ✅ badge, first-touched timestamps, exact ops | `via: git` diff over the intent→done window, ◐ "changed during session — not hook-verified" |
| Command history / local checks | recorded with exit codes | absent (or `shale note`) |
| Prompt transcript | captured, redacted, hashed | absent |
| Session ID | agent's own (UUID) | Shale-minted ULID |
| Card, coverage accounting, nudge | yes | yes — unchanged |

The merge is per-path, not per-session: hook evidence wins over git-derived
evidence for the same file (`render.writeFiles` precedence), so an agent with
*partial* hook coverage (Codex) gets ✅ where hooks saw the edit and ◐
elsewhere. Nothing anywhere requires hooks to succeed: capture exits 0 on all
errors, finalize falls back to git, the card renders the difference honestly
instead of failing. **Ship order follows confidence:** steering + CLI is GA
behavior for every agent on day one; each hook adapter ships only when its
fixtures are recorded and its trust UX is documented.

---

## 4. `shale init` redesign — least friction

### 4.1 Current behavior

`init_command.go` today: writes steering to detected instruction files, installs
Claude hooks **globally**, scaffolds `.shale/` + workflow, installs the pre-push
hook. Only Claude Code, only global, no detection beyond steering files.

### 4.2 Target behavior — repo-level by default, three modes

**Default (`shale init`): repo-level wiring for every agent that supports it.**
Always-written, checked-in, travels on clone:

| File | Covers | Written |
|---|---|---|
| `.shale/` scaffold + `config.yaml` | opt-in marker, privacy | always |
| `.github/workflows/shale.yml` | the card | always |
| steering block in `CLAUDE.md`, `AGENTS.md` | semantic tier, universal | always |
| `.claude/settings.json` (repo) | Claude Code **+ VS Code Copilot** | always |
| `.github/hooks/shale.json` | Copilot CLI **+ Copilot cloud agent + VS Code Copilot** | always |
| `.cursor/hooks.json` | Cursor | always |
| `.codex/hooks.json` | Codex CLI (user must trust via `/hooks` once) | always |
| `.git/hooks/pre-push` | finalize safety net | always (local, not committed) |

**Decision: all agents, unconditionally, no prompting.** The person running
`shale init` cannot know which agents future contributors use — in an open
source project, *nobody* can. Prompting "which agents?" would add friction to
answer a question that has no answerable form; detection against the
initializer's machine measures the wrong population (the audience for committed
files is future contributors, not the initializer). So every repo hook file is
written, always. This is safe because the files are doubly inert: the guarded
command (§4.6) no-ops without the `shale` binary, and `shale capture` fails
open on adapters that haven't shipped yet. They are small, namespaced,
reviewable, and cost nothing when unused — like `.editorconfig`, which nobody
detection-gates either.

`.claude/settings.json` is the single highest-leverage file: Claude Code reads
it **and** VS Code Copilot reads it (§2.1), so one committed file wires the two
most common agents. `.github/hooks/shale.json` adds the Copilot CLI and cloud
agent. The default install covers the entire matrix with zero machine-global
state.

**`--global`: opt into machine-wide wiring.** For developers who want capture
across *all* their repos without per-repo init. Here detection-gating IS
correct — the question is what's installed on *this* machine: write
`~/.claude/settings.json`, `~/.cursor/hooks.json`, `~/.codex/hooks.json`,
`~/.copilot/hooks/shale.json` only when the agent's home dir exists. Global
commands are unguarded (`shale capture <agent>` plain — the user just ran the
binary). The `.shale/` check still gates activation per repo (§3.2). Side
benefit for Codex: your own global hooks skip the per-repo trust review that
committed hooks require.

**`--hooks-only`: pre-push hook only** (existing fork-contributor path —
unchanged).

### 4.3 Guarded vs plain commands

Two command forms, by audience:

- **Committed (repo) files** — guarded:
  `command -v shale >/dev/null 2>&1 && shale capture <agent> || true`
  (POSIX; Codex entries also carry `commandWindows` with the `where`-based
  cmd.exe variant). These run on machines we know nothing about; silence is
  mandatory.
- **Global (`--global`) files** — plain: `shale capture <agent>`. The user just
  ran the binary on this machine; the guard would only obscure.

Idempotency detection accepts either form (substring match on
`shale capture <agent>`), so upgrading between forms never duplicates entries.

### 4.4 `shale doctor` follows

Implemented: doctor reports "Claude Code capture hooks installed (repo or
global)" and "multi-agent repo hook configs present"
(`.github/hooks/shale.json`, `.cursor/hooks.json`, `.codex/hooks.json`),
alongside the existing checks. A `✗` line names the exact fix, as today.

### 4.5 Output (as implemented)

```
$ shale init
  ✓ Steering prompt added      (CLAUDE.md, AGENTS.md)
  ✓ Wrote repo capture hooks   (.claude/settings.json, .github/hooks/shale.json,
                                .cursor/hooks.json, .codex/hooks.json
                                — all agents, inert without shale on PATH)
  ✓ Created .shale/            (committed; .shale/local/ gitignored)
  ✓ Wrote .github/workflows/shale.yml   (renders the card on PRs)
  ✓ Installed pre-push hook    (runs shale finalize)
  → Commit these files. Contributors and their agents are wired on clone.
```

### 4.6 The CLI-install gap (repo-level's one real cost)

Committed hook config travels to every contributor — but the `shale` binary
does not. A teammate who clones the repo and starts an agent has the hook config
but may not have the CLI. Left unhandled, a committed hook referencing a missing
binary errors on **every** tool-use event and makes the repo look broken to
people who never opted in. Three layers close the gap; use all three.

**Layer 1 — self-guarding hook commands (mandatory for committed hooks).**
Every committed hook command guards on the binary's presence so a missing CLI is
a *silent no-op*, not a per-event error:

```jsonc
// .claude/settings.json (committed)
"command": "command -v shale >/dev/null 2>&1 && shale capture claude-code"
```

The hook stays silently inert until the contributor installs `shale`, then
activates automatically — no errors, no noise, nothing to explain. This is the
difference between repo-level hooks being pleasant and being a support burden.
(Portability: this is the POSIX form. Where the agent's config exposes per-OS
command fields — VS Code Copilot's `windows`/`linux`/`osx` keys — emit a
`where shale >NUL 2>&1 && …` variant for Windows; Codex hooks are unix-only so
no variant is needed there.)

**Layer 2 — steering tells the agent to prompt the developer (no auto-install).**
The steering block is read by the *agent*, which detects the missing binary —
but it does **not** install anything itself. It surfaces a clear, actionable
message to the developer with the download URL. Add to `SteeringBlock`:

```
If `shale` is not on your PATH, do not try to install it yourself. Tell the
user: "Shale CLI is not installed. Download the latest release for your
platform from https://github.com/provasign/shale/releases/latest, unpack it,
and put the `shale` binary on your PATH." Then continue without it.
```

Two deliberate choices:

- **Prompt, don't auto-install.** An agent running an installer unprompted is a
  larger action than running the tool and rubs against Shale's
  no-surprise-network-calls posture (ADR D6). The developer stays in control of
  what lands on their machine; the agent's only job is to tell them precisely
  what to do and where.
- **GitHub Releases `latest`, not a package manager.** The canonical install URL
  is `https://github.com/provasign/shale/releases/latest` — it always resolves
  to the newest published release, with the per-platform
  `shale_{os}_{arch}.tar.gz` assets and their `.sha256` files. There is no
  working `brew` formula yet (the goreleaser homebrew-tap was never created), so
  Releases is the *only* real install path today. When a tap or other package
  manager ships, add it here; until then, do not advertise it.

**Layer 3 — existing safety nets (backstop + discovery).** Nothing new to build:

- the **pre-push hook already fails open** — `shale finalize --auto-commit ||
  echo …` means a missing binary at push time skips finalize and lets the push
  through (`initx/hooks.go`);
- the **PR card nudge** is the discovery surface — a PR with no evidence renders
  "No shale for this PR … `brew install shale && shale init`" (`render.Nudge`),
  so anyone who slips through every other layer still sees exactly what to do,
  on the PR itself.

Optionally, `shale init` can append a short install note to `CONTRIBUTING.md`
(idempotent, marker-fenced like the steering block) so the human-readable path
exists too — but layers 1–3 already make the binary-absent case correct and
quiet without it.

---

## 5. Distribution — making `shale` installable via Homebrew

The install URL cited in the steering block and the self-guarding hook check
(`command -v shale`) points to GitHub Releases today. Homebrew is the
lowest-friction install path on macOS and Linux; here is how to stand it up.

### 5.1 Create the tap repo

Create a public GitHub repo named **`provasign/homebrew-shale`** (Homebrew
requires the `homebrew-` prefix). Once it exists, users install with:

```sh
brew install provasign/shale/shale
# or equivalently:
brew tap provasign/shale && brew install shale
```

### 5.2 Formula file — `Formula/shale.rb`

Add this file to the tap repo. The four `sha256` values come from the
`.sha256` files attached to each GitHub release.

```ruby
class Shale < Formula
  desc "AI agent PR evidence — capture, verify, render"
  homepage "https://github.com/provasign/shale"
  version "0.1.6"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/provasign/shale/releases/download/v#{version}/shale_darwin_arm64.tar.gz"
      sha256 "<shale_darwin_arm64.tar.gz.sha256>"
    else
      url "https://github.com/provasign/shale/releases/download/v#{version}/shale_darwin_amd64.tar.gz"
      sha256 "<shale_darwin_amd64.tar.gz.sha256>"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/provasign/shale/releases/download/v#{version}/shale_linux_arm64.tar.gz"
      sha256 "<shale_linux_arm64.tar.gz.sha256>"
    else
      url "https://github.com/provasign/shale/releases/download/v#{version}/shale_linux_amd64.tar.gz"
      sha256 "<shale_linux_amd64.tar.gz.sha256>"
    end
  end

  def install
    bin.install "shale"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/shale version")
  end
end
```

### 5.3 Updating on each release

Until goreleaser is fully automated (see §5.4), updating is one commit to the
tap repo per release: bump `version` and the four `sha256` values. The values
are already published — each release asset has a companion
`shale_{os}_{arch}.tar.gz.sha256` file on the releases page.

### 5.4 Automating via goreleaser (future)

The `.goreleaser.yml` in this repo already has a `brews:` section targeting the
tap. It is currently blocked by `draft: true` in the release config and the tap
repo not existing. Once the tap repo is created:

1. Remove (or conditionalize) `draft: true` in `.goreleaser.yml`
2. Ensure `brews[].repository.name` is set to `homebrew-shale`
3. Add a `HOMEBREW_TAP_TOKEN` secret to the shale repo (a PAT with `repo` scope
   on `provasign/homebrew-shale`)
4. The goreleaser GitHub Actions workflow will update the formula automatically
   on each tag push — `version` and all four `sha256` values written for you

Until automation is wired, the hand-update in §5.3 takes about two minutes.

### 5.5 Steering + nudge update (required with the tap)

When the tap is live, update two places so the install instructions are
consistent everywhere:

- **`SteeringBlock`** in `internal/initx/steering.go` — replace the Releases
  URL with `brew install provasign/shale/shale` as the primary instruction and
  keep the Releases URL as the fallback for non-macOS/Linux.
- **`render.Nudge()`** in `internal/render/render.go` — currently shows
  `brew install shale && shale init` (broken — no formula exists yet); once the
  tap ships, change to `brew install provasign/shale/shale && shale init`.
  Until then do not change it to advertise a tap that doesn't exist.

---

## 6. Implementation notes for the adapters

- **One parser per agent** in `internal/capture/`, each total and fail-open,
  fixtures recorded under `testdata/<agent>/` (mirror `testdata/claude-code/`).
  Confirm exact payload field names from a real session before finalizing —
  docs drift; fixtures don't.
- **Normalize once.** A small helper to fold PascalCase/camelCase event names
  and `session_id`/`sessionId`/`conversation_id` keeps each parser a thin map.
- **Wiring functions** mirror `InstallClaudeHooks`: additive, idempotent, never
  clobber foreign config, detect-by-marker for "already present".
- **Session-id shape.** Hook agents supply their own id (UUID-ish); `displayID`
  in `render.go` already truncates UUIDs and leaves ULIDs (the semantic-tier
  fallback id) intact — no change needed.
- **Drift is contained.** Hook APIs move. When one breaks, only that agent's
  mechanical tier degrades; steering + `shale done --model/--tokens` + the git
  fallback keep the card honest (ADR D4 tier 1 & 3).
