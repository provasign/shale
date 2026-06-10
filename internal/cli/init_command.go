package cli

import (
	"flag"
	"fmt"
	"io"

	"github.com/provasign/shale/internal/initx"
	"github.com/provasign/shale/internal/store"
)

// cmdInit is the 2-command setup (docs/01-product.md §3.1). Idempotent, and
// never destroys user content: steering blocks append, hook config merges,
// scaffold skips existing files, foreign pre-push hooks are left alone.
func cmdInit(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(stderr)
	privacy := fs.String("privacy", store.PrivacyRedacted, "prompt privacy: full | redacted | hash-only")
	hooksOnly := fs.Bool("hooks-only", false, "only install the pre-push hook (fork contributor path)")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	switch *privacy {
	case store.PrivacyFull, store.PrivacyRedacted, store.PrivacyHashOnly:
	default:
		fmt.Fprintf(stderr, "shale init: invalid --privacy %q (full|redacted|hash-only)\n", *privacy)
		return 1
	}

	root := repoRoot()
	ok := true

	if !*hooksOnly {
		if written, err := initx.WriteSteering(root); err != nil {
			fmt.Fprintln(stderr, "shale init: steering:", err)
			ok = false
		} else if len(written) > 0 {
			fmt.Fprintf(stdout, "  ✓ Steering prompt added      (%s)\n", join(written))
		} else {
			fmt.Fprintln(stdout, "  ✓ Steering prompt            (already present)")
		}

		if path, err := initx.ClaudeSettingsPath(); err == nil {
			changed, herr := initx.InstallClaudeHooks(path)
			switch {
			case herr != nil:
				fmt.Fprintln(stderr, "shale init: claude hooks:", herr)
				ok = false
			case changed:
				fmt.Fprintf(stdout, "  ✓ Installed capture hooks    (%s)\n", path)
			default:
				fmt.Fprintln(stdout, "  ✓ Capture hooks              (already present)")
			}
		}

		if written, err := initx.Scaffold(root, *privacy); err != nil {
			fmt.Fprintln(stderr, "shale init: scaffold:", err)
			ok = false
		} else if len(written) > 0 {
			fmt.Fprintln(stdout, "  ✓ Created .shale/            (committed; .shale/local/ gitignored)")
			fmt.Fprintln(stdout, "  ✓ Wrote .github/workflows/shale.yml   (renders the card on PRs)")
		} else {
			fmt.Fprintln(stdout, "  ✓ Scaffold                   (already present)")
		}
	}

	skipped, err := initx.InstallPrePushHook(root)
	switch {
	case err != nil:
		fmt.Fprintln(stderr, "shale init: pre-push hook:", err)
		ok = false
	case skipped:
		fmt.Fprintln(stdout, "  ! Pre-push hook skipped      (existing hook found — add `shale finalize --auto-commit` to it)")
	default:
		fmt.Fprintln(stdout, "  ✓ Installed pre-push hook    (runs shale finalize)")
	}

	if !ok {
		return 1
	}
	fmt.Fprintln(stdout, "  → Commit these files and open your next PR as usual. Done.")
	return 0
}

// cmdDoctor prints one line per check, with the fix when failing (plan C2).
func cmdDoctor(stdout io.Writer) int {
	root := repoRoot()
	failed := 0
	for _, c := range initx.Doctor(root) {
		if c.OK {
			fmt.Fprintf(stdout, "  ✓ %s\n", c.Name)
		} else {
			failed++
			fmt.Fprintf(stdout, "  ✗ %s — %s\n", c.Name, c.Fix)
		}
	}
	if failed > 0 {
		fmt.Fprintf(stdout, "%d problem(s) found\n", failed)
		return 1
	}
	fmt.Fprintln(stdout, "all checks passed")
	return 0
}

func join(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += ", "
		}
		out += p
	}
	return out
}
