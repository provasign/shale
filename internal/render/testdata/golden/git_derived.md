<!-- shale-card -->
## <img src="https://provasign.dev/assets/images/logo-icon.png" width="20" height="20" alt=""> Shale · 1 session · claude-code (claude-fable-5)
claude-fable-5 · 47k tokens · ~$0.47 · 3 iterations · 38 min

> ℹ️ Session `k1l2m3n4o5` only has git fallback file evidence: Shale saw files change while the session was active, but no agent hook reported those edits. Token and command totals may be incomplete.

### Intent
> **Add rate limiting to the login endpoint**
>
> Brute force attempts observed in prod logs. Redis counter, 10 req/min per IP.

*Declared 2026-06-09 14:02 UTC · session `k1l2m3n4o5` · agent `claude-code` · model `claude-fable-5` · 47k tokens · ~$0.47 · 3 iterations · 38 min · transcript `sha256:4be91d03…`*

### Completion
> **`k1l2m3n4o5`** · Redis-backed rate limiter implemented. In-memory fallback added.

### Changed files (5) — 1 with evidence · 4 without session evidence

*Legend: ✅ hook event = an agent hook reported the file edit; ◐ git fallback = the file changed while that session was active, but no hook event was recorded; — = no session evidence matched the PR file.*
| Session ID | Evidence | File | Notes |
|---|---|---|---|
| `k1l2m3n4o5` | ◐ git fallback | `internal/auth/login.go` | changed while session was active; no agent hook event recorded · **sensitive path: auth/crypto path** |
| `—` | — | `.github/workflows/deploy.yml` | **sensitive path: CI config** |
| `—` | — | `go.mod` | **sensitive path: dependency manifest** |
| `—` | — | `internal/auth/ratelimit.go` | **sensitive path: auth/crypto path** |
| `—` | — | `internal/auth/ratelimit_test.go` | **sensitive path: auth/crypto path** |

### Checks recorded locally
| Session ID | Check | Result | When |
|---|---|---|---|
| `k1l2m3n4o5` | `gitleaks detect --no-banner` | ✅ passed | 2026-06-09 14:31 UTC |
| `k1l2m3n4o5` | `go test ./internal/auth/...` | ✅ passed | 2026-06-09 14:33 UTC |

*Advisory — CI is authoritative.*
