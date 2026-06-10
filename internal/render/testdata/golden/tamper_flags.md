<!-- shale-card -->
## 🧾 Shale · 1 session · claude-code (claude-fable-5)
claude-fable-5 · 47k tokens · ~$0.47 · 3 iterations · 38 min

> ⚠️ transcript hash mismatch for session a1b2c3 — treat intent text as unverified

> ⚠️ shale a1b2c3 edited after capture

### Intent
> **Add rate limiting to the login endpoint**
>
> Brute force attempts observed in prod logs. Redis counter, 10 req/min per IP.

*Declared 2026-06-09 14:02 · session `a1b2c3d4e5` · transcript `sha256:4be91d03…`*

### Completion
> Redis-backed rate limiter implemented. In-memory fallback added.

### Changed files (5) — 3 with evidence · 2 untracked
| File | Evidence | Notes |
|---|---|---|
| `internal/auth/ratelimit.go` | ✅ a1b2c3d4e5 | **sensitive path: auth/crypto path** |
| `internal/auth/ratelimit_test.go` | ✅ a1b2c3d4e5 | **sensitive path: auth/crypto path** |
| `internal/auth/login.go` | ✅ a1b2c3d4e5 | **sensitive path: auth/crypto path** |
| `go.mod` | — | **sensitive path: dependency manifest** |
| `.github/workflows/deploy.yml` | — | **sensitive path: CI config** |

### Checks recorded locally
| Check | Result | When |
|---|---|---|
| `gitleaks detect --no-banner` | ✅ passed | 14:31 |
| `go test ./internal/auth/...` | ✅ passed | 14:33 |

*Advisory — CI is authoritative.*
