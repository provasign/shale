<!-- shale-card -->
## 🧾 Shale · 1 session · claude-code (claude-fable-5)
claude-fable-5 · 47k tokens · ~$0.47 · 3 iterations · 38 min

### Intent
> **&lt;img src=x onerror=alert(1)&gt; @​everyone fix #​1 [click]​(https://evil.example)**
>
> &lt;script&gt;steal()&lt;/script&gt; ping @​maintainer

*Declared 2026-06-09 14:02 · session `evil123456` · model `claude-fable-5` · transcript `sha256:4be91d03…`*

### Completion
> done &amp; dusted &lt;iframe src=evil&gt;

### Changed files (5) — 2 with evidence · 3 untracked
| File | Session ID | Notes |
|---|---|---|
| `internal/auth/ratelimit.go` | — | **sensitive path: auth/crypto path** |
| `internal/auth/ratelimit_test.go` | ✅ evil123456 | **sensitive path: auth/crypto path** |
| `internal/auth/login.go` | ✅ evil123456 | **sensitive path: auth/crypto path** |
| `go.mod` | — | **sensitive path: dependency manifest** |
| `.github/workflows/deploy.yml` | — | **sensitive path: CI config** |

### Checks recorded locally
| Check | Result | When |
|---|---|---|
| `echo '@​all' #​42` | ✅ passed | 14:31 |
| `go test ./internal/auth/...` | ✅ passed | 14:33 |

*Advisory — CI is authoritative.*
