<!-- shale-card -->
## 🧾 Shale · 2 sessions · claude-code (claude-fable-5, claude-sonnet-4-6)
94k tokens · ~$0.94 · 6 iterations · 77 min

### Intent
> **Add rate limiting to the login endpoint**
>
> Brute force attempts observed in prod logs. Redis counter, 10 req/min per IP.

*Declared 2026-06-09 14:02 UTC · session `a1b2c3d4e5` · agent `claude-code` · model `claude-fable-5` · 47k tokens · ~$0.47 · 3 iterations · 38 min · transcript `sha256:4be91d03…`*

> **Add tests for the fallback path**

*Declared 2026-06-09 14:02 UTC · session `f6g7h8i9j0` · agent `claude-code` · model `claude-sonnet-4-6` · 47k tokens · ~$0.47 · 3 iterations · 38 min · transcript `sha256:4be91d03…`*

### Completion
> **`a1b2c3d4e5`** · Redis-backed rate limiter implemented. In-memory fallback added.
> **`f6g7h8i9j0`** · Redis-backed rate limiter implemented. In-memory fallback added.

### Changed files (5) — 3 with evidence · 2 without session evidence

*Legend: ✅ hook event = an agent hook reported the file edit; ◐ git fallback = the file changed while that session was active, but no hook event was recorded; — = no session evidence matched the PR file.*
| Session ID | Evidence | File | Notes |
|---|---|---|---|
| `a1b2c3d4e5` | ✅ hook event | `internal/auth/login.go` | **sensitive path: auth/crypto path** |
| `a1b2c3d4e5` | ✅ hook event | `internal/auth/ratelimit.go` | **sensitive path: auth/crypto path** |
| `a1b2c3d4e5` | ✅ hook event | `internal/auth/ratelimit_test.go` | **sensitive path: auth/crypto path** |
| `—` | — | `.github/workflows/deploy.yml` | **sensitive path: CI config** |
| `—` | — | `go.mod` | **sensitive path: dependency manifest** |

### Checks recorded locally
| Session ID | Check | Result | When |
|---|---|---|---|
| `a1b2c3d4e5` | `gitleaks detect --no-banner` | ✅ passed | 2026-06-09 14:31 UTC |
| `a1b2c3d4e5` | `go test ./internal/auth/...` | ✅ passed | 2026-06-09 14:33 UTC |
| `f6g7h8i9j0` | `gitleaks detect --no-banner` | ✅ passed | 2026-06-09 14:31 UTC |
| `f6g7h8i9j0` | `go test ./internal/auth/...` | ✅ passed | 2026-06-09 14:33 UTC |

*Advisory — CI is authoritative.*
