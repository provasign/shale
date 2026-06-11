<!-- shale-card -->
## <img src="https://provasign.dev/assets/images/logo-icon.png" width="20" height="20" alt=""> Shale · 1 session · claude-code (claude-fable-5)
claude-fable-5 · 47k tokens · ~$0.47 · 3 iterations · 38 min

### Intent
> **&lt;img src=x onerror=alert(1)&gt; @​everyone fix #​1 [click]​(https://evil.example)**
>
> &lt;script&gt;steal()&lt;/script&gt; ping @​maintainer

*Declared 2026-06-09 14:02 UTC · session `evil123456` · agent `claude-code` · model `claude-fable-5` · 47k tokens · ~$0.47 · 3 iterations · 38 min · transcript `sha256:4be91d03…`*

### Completion
> **`evil123456`** · done &amp; dusted &lt;iframe src=evil&gt;

### Changed files (5) — 2 with evidence · 3 without session evidence

*Legend: ✅ hook event = an agent hook reported the file edit; ◐ git fallback = the file changed while that session was active, but no hook event was recorded; — = no session evidence matched the PR file.*
| Session ID | Evidence | File | Notes |
|---|---|---|---|
| `evil123456` | ✅ hook event | `internal/auth/login.go` | **sensitive path: auth/crypto path** |
| `evil123456` | ✅ hook event | `internal/auth/ratelimit_test.go` | **sensitive path: auth/crypto path** |
| `—` | — | `.github/workflows/deploy.yml` | **sensitive path: CI config** |
| `—` | — | `go.mod` | **sensitive path: dependency manifest** |
| `—` | — | `internal/auth/ratelimit.go` | **sensitive path: auth/crypto path** |

### Checks recorded locally
| Session ID | Check | Result | When |
|---|---|---|---|
| `evil123456` | `echo '@​all' #​42` | ✅ passed | 2026-06-09 14:31 UTC |
| `evil123456` | `go test ./internal/auth/...` | ✅ passed | 2026-06-09 14:33 UTC |

*Advisory — CI is authoritative.*
