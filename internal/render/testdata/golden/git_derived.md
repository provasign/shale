<!-- shale-card -->
## 🧾 Shale · 1 session · claude-code (claude-fable-5)
claude-fable-5 · 47k tokens · ~$0.47 · 3 iterations · 38 min

### Intent
> **Add rate limiting to the login endpoint**
>
> Brute force attempts observed in prod logs. Redis counter, 10 req/min per IP.

*Declared 2026-06-09 14:02 · session `k1l2m3` · 14 prompts · transcript hash `sha256:4be91d03…`*

### Completion
> Redis-backed rate limiter implemented. In-memory fallback added.

### Changed files (5) — 1 seen in agent sessions, 4 not
| File | Agent session | Notes |
|---|---|---|
| `internal/auth/ratelimit.go` | ⚠️ none | **sensitive path: auth/crypto path** |
| `internal/auth/ratelimit_test.go` | ⚠️ none | **sensitive path: auth/crypto path** |
| `internal/auth/login.go` | ◐ k1l2m3 | changed during session — not hook-verified · **sensitive path: auth/crypto path** |
| `go.mod` | ⚠️ none | **sensitive path: dependency manifest** |
| `.github/workflows/deploy.yml` | ⚠️ none | **sensitive path: CI config** |

### Checks recorded locally
| Check | Result | When |
|---|---|---|
| `gitleaks detect --no-banner` | ✅ passed | 14:31 |
| `go test ./internal/auth/...` | ✅ passed | 14:33 |

*Recorded from the agent session — advisory only. CI remains authoritative.*

### Coverage gaps
⚠️ 4 changed files have no session evidence. They may be hand-edits or
changes from an uninstrumented tool.
