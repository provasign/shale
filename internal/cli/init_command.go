package cli

import (
	"flag"
	"fmt"
	"io"

	"github.com/provasign/shale/internal/initx"
	"github.com/provasign/shale/internal/store"
)

// cmdInit is the 2-command setup (docs/product.md §2, redesigned per
// docs/product.md §4). Default mode is repo-level: everything it
// writes is committed and travels on clone — steering, hook config for ALL
// agents (no detection; the initializer can't know what future contributors
// use, and the guarded commands are inert without the binary), scaffold,
// workflow. --global additionally wires machine-wide hooks, detection-gated.
// Idempotent, and never destroys user content: steering blocks append, hook
// config merges, scaffold skips existing files, foreign pre-push hooks are
// left alone.
func cmdInit(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(stderr)
	privacy := fs.String("privacy", store.PrivacyRedacted, "prompt privacy: full | redacted | hash-only")
	hooksOnly := fs.Bool("hooks-only", false, "only install the pre-push hook (fork contributor path)")
	global := fs.Bool("global", false, "also wire machine-wide hooks for agents installed on this machine")
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

		if written, err := initx.InstallRepoHooks(root); err != nil {
			fmt.Fprintln(stderr, "shale init: repo hooks:", err)
			ok = false
		} else if len(written) > 0 {
			fmt.Fprintf(stdout, "  ✓ Wrote repo capture hooks   (%s — all agents, inert without shale on PATH)\n", join(written))
		} else {
			fmt.Fprintln(stdout, "  ✓ Repo capture hooks         (already present)")
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

	if *global {
		if written, err := initx.InstallGlobalHooks(); err != nil {
			fmt.Fprintln(stderr, "shale init: global hooks:", err)
			ok = false
		} else if len(written) > 0 {
			fmt.Fprintf(stdout, "  ✓ Installed global capture hooks (%s)\n", join(written))
		} else {
			fmt.Fprintln(stdout, "  ✓ Global capture hooks       (already present, or no agents detected)")
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
	fmt.Fprintln(stdout, "  → Commit these files. Contributors and their agents are wired on clone.")
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
