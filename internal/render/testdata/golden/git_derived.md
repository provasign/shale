<!-- shale-card -->
## 🧾 Shale · 1 session · claude-code (claude-fable-5)
claude-fable-5 · 47k tokens · ~$0.47 · 3 iterations · 38 min

> ℹ️ Hook validation was not observed for session `k1l2m3n4o5`; file evidence is git-derived and token/command telemetry may be incomplete.

### Intent
> **Add rate limiting to the login endpoint**
>
> Brute force attempts observed in prod logs. Redis counter, 10 req/min per IP.

*Declared 2026-06-09 14:02 · session `k1l2m3n4o5` · model `claude-fable-5` · transcript `sha256:4be91d03…`*

### Completion
> Redis-backed rate limiter implemented. In-memory fallback added.

### Changed files (5) — 1 with evidence · 4 untracked
| File | Session ID | Notes |
|---|---|---|
| `internal/auth/ratelimit.go` | — | **sensitive path: auth/crypto path** |
| `internal/auth/ratelimit_test.go` | — | **sensitive path: auth/crypto path** |
| `internal/auth/login.go` | ◐ k1l2m3n4o5 | changed during session — not hook-verified · **sensitive path: auth/crypto path** |
| `go.mod` | — | **sensitive path: dependency manifest** |
| `.github/workflows/deploy.yml` | — | **sensitive path: CI config** |

### Checks recorded locally
| Check | Result | When |
|---|---|---|
| `gitleaks detect --no-banner` | ✅ passed | 14:31 |
| `go test ./internal/auth/...` | ✅ passed | 14:33 |

*Advisory — CI is authoritative.*
