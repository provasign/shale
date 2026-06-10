# Shale format — spec v0 (draft)

The shale file is the product's contract: agents/adapters write it, the
Action renders it, future hosted tooling (later) ingests it, and third parties
are invited to implement it. It must be readable by a human in a code review **and**
strictly parseable by machine. Spec v1 (MVP 2) will be published with a JSON
Schema in `spec/` and a conformance test suite; v0 below is the build target
for MVP 1.

Media type: `application/vnd.shale+yaml;v=0` ·
Predicate URI (for future in-toto wrapping by a hosted notary):
`https://provasign.dev/shale/v0`

## 1. File placement

```
.shale/<session-id>.yaml             # one file per agent session, append-only
.shale/transcripts/<session-id>.md   # redacted transcript (optional by privacy mode)
```

`<session-id>` is the agent-supplied session UUID when available, else a
generated ULID. File names never change after first commit.

## 2. Schema (v0, YAML)

```yaml
shale_version: "0"
id: "01J9ZK7Q4N8WPXG2"            # session id (agent-native) or ULID
created_at: "2026-06-09T14:02:11Z" # first event in session (UTC, RFC 3339)
finalized_at: "2026-06-09T14:41:03Z"

agent:
  tool: "claude-code"              # adapter name, lowercase kebab
  tool_version: "2.1.0"            # if discoverable, else omit
  model: "claude-fable-5"          # as reported by the agent; never guessed
  adapter_version: "0.3.0"         # shale's own adapter version

repo:
  root_hint: "my-repo"             # basename only — never absolute paths
  branch: "feat/login-rate-limit"  # branch at finalize time

privacy: "redacted"                # full | redacted | hash-only

intent:
  # Written by the AGENT via `shale intent "<title>" [--body "..."]` BEFORE
  # the first file edit, steered by the CLAUDE.md / AGENTS.md / .cursorrules
  # block written by `shale init`. Never inferred from prompts — if absent,
  # the card shows "no intent declared", never garbage.
  title: "Add rate limiting to the login endpoint"
  body: |
    Brute force attempts observed in prod logs. Redis counter, 10 req/min
    per IP, return 429 with Retry-After header. Tests required.
  title_sha256: "9f1c2ab4…"        # hash of UNredacted title (proves capture)
  body_sha256: "3d7e91f2…"         # hash of UNredacted body; omit if body absent
  declared_at: "2026-06-09T14:02:11Z"  # timestamp of `shale intent` call
  prompt_count: 14                 # total user prompts in session

completion:
  # Written by the AGENT via `shale done [--note "..."] [--tokens-in N ...]`
  # AFTER the work is complete, before asking for review. Steered by the same
  # block as intent. `shale finalize` (pre-push hook) is the mechanical
  # safety net; `shale done` is the semantic act.
  note: "Redis-backed rate limiter implemented. In-memory fallback added when Redis unavailable. 3 files changed, 12 tests added."
  model: "claude-fable-5"          # as reported by the agent at done-time
  tokens_in: 32000                 # prompt tokens for session
  tokens_out: 15000                # completion tokens for session
  tokens_total: 47000              # sum (shale computes if not provided)
  cost_usd: 0.47                   # computed by shale from built-in pricing table
  iterations: 3                    # prompt-response cycles
  # duration_minutes: derived automatically from intent.declared_at → finalized_at

transcript:
  path: "transcripts/01J9ZK7Q4N8WPXG2.md"   # omit in hash-only mode
  sha256: "4be91d03…"              # hash of the redacted transcript file as committed
  kind: "prompts"                  # prompts (default) | full
  # "prompts": user prompts + timestamps + one-line turn summaries ONLY —
  # never tool outputs or agent reasoning. Keeps committed transcripts ~KBs
  # (repo-growth rule, ADR D3a). "full" is an explicit opt-in.

files:                             # union of agent file touches, repo-relative
  - path: "internal/auth/ratelimit.go"
    ops: ["write"]                 # write | edit | delete
    via: "hook"                    # hook (verified by an agent hook adapter)
                                   # | git (filled by finalize from git diff
                                   #   over the intent→done window — ADR D4
                                   #   tier 3, used when the agent has no hook
                                   #   adapter, e.g. Copilot)
    first_touched: "2026-06-09T14:05:02Z"  # omit when via: git
  - path: "internal/auth/login.go"
    ops: ["edit"]
    via: "hook"

commands:                          # agent-invoked commands (feeds "checks recorded")
  - cmd: "go test ./internal/auth/..."
    exit_code: 0
    at: "2026-06-09T14:33:40Z"
    classified: "test"             # test | lint | scan | build | other (heuristic)
  - cmd: "gitleaks detect --no-banner"
    exit_code: 0
    at: "2026-06-09T14:31:12Z"
    classified: "scan"

notes: []                          # manual `shale note` entries, timestamped

redactions: 2                      # count of redaction hits (never the content)
```

## 3. Rules

1. **Schema-versioned, forward-only.** Renderers must accept unknown fields and
   reject unknown *major* versions. `shale_version` bumps only on breaking
   change.
2. **Append-only.** A finalized, committed shale is never edited. Corrections
   are new `notes` in a new finalize pass before push, or a follow-up session.
3. **No absolute paths, no usernames, no hostnames, no env values.** The file
   is destined for a public PR.
4. **Honest provenance only.** `model` and `tool` come from the agent's own
   hook payload or `shale done` call, never inferred. If unknown, omit — the
   card renders "unknown", and absence stays explicit.
5. **Intent is always agent-declared, never inferred.** `intent.title` is set
   only by an explicit `shale intent` call. If absent, the card renders
   "no intent declared" — never a prompt, never a heuristic, never garbage.
6. **Hashes prove, text persuades.** `title_sha256` + `body_sha256` +
   `transcript.sha256` let anyone detect post-hoc edits without any server
   or signature. Signatures (DSSE/Sigstore) are an enclosing layer added by
   hosted-tier tooling — the shale body stays unsigned and stable.
7. **Renderer contract:** the card must be derivable from shale files + the
   PR diff alone. Any field that would require network access to interpret is
   spec-invalid.
8. **`cost_usd` is computed, never stored from outside.** Shale computes it
   from `tokens_*` + a versioned built-in pricing table. Callers pass tokens;
   Shale owns the arithmetic. The pricing table version is recorded in the
   file so old costs remain reproducible after price changes.
9. **Evidence provenance is labeled, never blended.** Every `files[]` entry
   carries `via: hook` (recorded by an agent hook adapter at edit time) or
   `via: git` (derived by finalize from the diff over the session window).
   The renderer must distinguish them — a git-derived file list is honest
   evidence of *what changed during the session*, not of *what the agent
   touched*.

## 4. Versioning roadmap

- **v0 (MVP 1):** the schema above; YAML only; validated by Go structs.
- **v1 (MVP 2):** published JSON Schema in `spec/shale.v1.schema.json`;
  `shale verify`; conformance fixtures; CHANGELOG discipline; announce as an
  open spec others can emit (this is the cross-agent neutrality play).
- **v2 (with the hosted-notary bridge):** stable in-toto predicate wrapping; explicit
  `checks[]` result objects contributed by scanner integrations.
