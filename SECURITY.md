# Security Policy

## Reporting a vulnerability

Please use GitHub's **private vulnerability reporting**: go to the
[Security tab](https://github.com/provasign/shale/security) of this repo and
click "Report a vulnerability". Do not open a public issue for anything you
believe is exploitable.

You can expect an acknowledgment within a few days. Coordinated disclosure
is appreciated; we'll credit reporters in the release notes unless you ask
otherwise.

## Supported versions

The latest release receives security fixes. There are no maintained older
release lines — upgrading is always the fix path (`brew upgrade shale`).

## Security model (what's worth probing)

Shale's security posture is documented in
[docs/product.md §5–§6](docs/product.md). The load-bearing properties, in
order of how much we'd like to know if you can break them:

1. **The render workflow never executes PR code.** It runs on
   `pull_request_target` with a write-capable token and is safe *only*
   because it reads everything through the API — there is no checkout. If
   you find any path by which PR-controlled content reaches execution in
   that workflow, report it immediately.
2. **All evidence text is treated as attacker-controlled.** Everything
   shale-derived passes a sanitizer before entering the PR card (HTML
   escaped, `@mentions`/`#refs` neutralized, link syntax broken, control
   characters stripped). A bypass that gets markup, mentions, or links
   through is a vulnerability.
3. **No network from laptop code paths** — capture, finalize, and init never
   touch the network (enforced by test). A dependency change that breaks
   this property is a vulnerability.
4. **Secrets must not reach committed evidence.** Gitignored paths are
   dropped, and intent/notes/commands pass secret redaction. A realistic
   payload that carries a credential into `.shale/*.yaml` is a
   vulnerability — please include the (defanged) payload shape.

## Explicit non-goals

Evidence is **tamper-evident, not tamper-proof** — edits after capture are
flagged on the card, but wholesale fabrication by a determined author with
laptop access is out of scope by design, and the card never claims
otherwise. Reports about fabricating evidence wholesale will be closed as
working-as-documented.
