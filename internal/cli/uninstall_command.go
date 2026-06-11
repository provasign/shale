package cli

import (
	"flag"
	"fmt"
	"io"

	"github.com/provasign/shale/internal/initx"
)

// cmdUninstall reverses `shale init`, mirroring its never-destroy rule:
// only what Shale wrote is removed. Default scope is this machine (pre-push
// hook, global agent hooks, local working state) — the committed repo
// surface belongs to the team and needs --repo plus a commit.
func cmdUninstall(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("uninstall", flag.ContinueOnError)
	fs.SetOutput(stderr)
	repo := fs.Bool("repo", false, "also remove the committed repo surface (steering, hook configs, workflow, .shale/) — commit the result")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	root := repoRoot()
	ok := true

	removed, chained, err := initx.RemovePrePushHook(root)
	switch {
	case err != nil:
		fmt.Fprintln(stderr, "shale uninstall: pre-push hook:", err)
		ok = false
	case removed:
		fmt.Fprintln(stdout, "  ✓ Removed pre-push hook")
	case chained:
		fmt.Fprintln(stdout, "  ! Pre-push hook is yours, not Shale's — remove the `shale finalize --auto-commit` line from it manually")
	default:
		fmt.Fprintln(stdout, "  ✓ Pre-push hook              (none installed)")
	}

	if changed, err := initx.RemoveGlobalHooks(); err != nil {
		fmt.Fprintln(stderr, "shale uninstall: global hooks:", err)
		ok = false
	} else if len(changed) > 0 {
		fmt.Fprintf(stdout, "  ✓ Removed global capture hooks (%s)\n", join(changed))
	} else {
		fmt.Fprintln(stdout, "  ✓ Global capture hooks       (none installed)")
	}

	if removed, err := initx.RemoveLocalState(root); err != nil {
		fmt.Fprintln(stderr, "shale uninstall: local state:", err)
		ok = false
	} else if removed {
		fmt.Fprintln(stdout, "  ✓ Removed .shale/local/      (raw events and prompts)")
	} else {
		fmt.Fprintln(stdout, "  ✓ Local state                (already clean)")
	}

	if *repo {
		if changed, err := initx.RemoveSteering(root); err != nil {
			fmt.Fprintln(stderr, "shale uninstall: steering:", err)
			ok = false
		} else if len(changed) > 0 {
			fmt.Fprintf(stdout, "  ✓ Removed steering blocks    (%s)\n", join(changed))
		} else {
			fmt.Fprintln(stdout, "  ✓ Steering blocks            (none found)")
		}

		if changed, err := initx.RemoveRepoHooks(root); err != nil {
			fmt.Fprintln(stderr, "shale uninstall: repo hooks:", err)
			ok = false
		} else if len(changed) > 0 {
			fmt.Fprintf(stdout, "  ✓ Removed repo capture hooks (%s)\n", join(changed))
		} else {
			fmt.Fprintln(stdout, "  ✓ Repo capture hooks         (none found)")
		}

		if removed, err := initx.RemoveScaffold(root); err != nil {
			fmt.Fprintln(stderr, "shale uninstall: scaffold:", err)
			ok = false
		} else if len(removed) > 0 {
			fmt.Fprintf(stdout, "  ✓ Removed evidence surface   (%s)\n", join(removed))
		} else {
			fmt.Fprintln(stdout, "  ✓ Evidence surface           (none found)")
		}
		initx.PruneEmptyDirs(root)
		fmt.Fprintln(stdout, "  → Commit the removal so it reaches your team. Past PR cards stay in their PR history.")
	} else {
		fmt.Fprintln(stdout, "  → Committed repo files were left for your team. `shale uninstall --repo` removes those too.")
	}

	fmt.Fprintln(stdout, "  → Remove the binary: `brew uninstall shale` (or delete it from your PATH).")
	if !ok {
		return 1
	}
	return 0
}
