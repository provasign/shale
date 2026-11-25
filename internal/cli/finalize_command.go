package cli

import (
	"flag"
	"fmt"
	"github.com/provasign/shale/internal/store"
	"io"
	"path/filepath"

	"github.com/provasign/shale/internal/fold"
	"github.com/provasign/shale/internal/gitx"
	"github.com/provasign/shale/internal/initx"
)

// cmdFinalize folds session events into committed shale YAML; safe to run
// manually. Pre-push hooks from before v0.2.3 call it with --auto-commit. Fail-open:
// finalize problems must never block a push (the hook script also exits 0).
func cmdFinalize(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("finalize", flag.ContinueOnError)
	fs.SetOutput(stderr)
	autoCommit := fs.Bool("auto-commit", false, "commit the written shale files (ADR D3)")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	root := repoRoot()
	// Only the pre-push hook an earlier release wrote calls this with
	// --auto-commit as a matter of course. Committing there strands the
	// commit, so replace that hook and do what the current one does.
	if *autoCommit && initx.IsOldPrePushHook(root) {
		if noteHookUpgrade(root, stderr) {
			fmt.Fprintln(stderr, "shale: nothing was committed; run `shale finalize --auto-commit` again if you meant to commit the evidence now")
			return cmdPrepush(stderr)
		}
	}
	cfg := initx.LoadConfig(root)
	res, err := fold.Run(fold.Options{RepoRoot: root, Privacy: cfg.Privacy})
	if err != nil {
		fmt.Fprintln(stderr, "shale finalize:", err)
		return 1
	}
	if len(res.Written) == 0 {
		fmt.Fprintln(stdout, "shale: nothing to finalize")
		return 0
	}
	fmt.Fprintf(stdout, "shale: finalized %d file(s)\n", len(res.Written))
	// Say whose evidence this is before it is committed: a push publishes
	// every agent's sessions folded here, not only the pusher's.
	for _, p := range res.Written {
		fmt.Fprintf(stdout, "  %s  %s\n", p, describeRecord(filepath.Join(root, p)))
	}
	if *autoCommit {
		committed, err := gitx.AutoCommit(root, res.Written, "chore(shale): session evidence")
		if err != nil {
			fmt.Fprintln(stderr, "shale finalize: auto-commit:", err)
			return 1
		}
		if committed {
			// Whether a commit created in a pre-push hook rides that same push
			// is git-internal and inconsistent (observed git 2.50: included
			// when the push transfers something, stranded when the push is
			// computed up-to-date; file:// remotes differ from https). Git's
			// status line and the remote-tracking ref are computed pre-hook
			// either way, so the user can't tell from the push output. Say
			// the only thing that is true in every case.
			fmt.Fprintln(stdout, "shale: evidence committed — if the push in flight does not include it, it ships with your next push")
		}
	}
	return 0
}

// describeRecord names an evidence file's agent and request, for the list
// finalize prints.
func describeRecord(path string) string {
	s, err := store.Load(path)
	if err != nil {
		return ""
	}
	tool := s.Agent.Tool
	if tool == "" {
		tool = "cli"
	}
	what := "no request declared"
	if s.Intent != nil && s.Intent.Title != "" {
		what = fmt.Sprintf("%q", s.Intent.Title)
	}
	return tool + " · " + what
}

// noteHookUpgrade replaces a pre-push hook an earlier release wrote (see
// initx.UpgradeOldHooks) and says so on w, when w is not nil. Reports
// whether it replaced one.
func noteHookUpgrade(root string, w io.Writer) bool {
	upgraded, err := initx.UpgradeOldHooks(root)
	if !upgraded {
		return false
	}
	if w != nil {
		fmt.Fprintf(w, "shale: replaced the pre-push hook an earlier Shale wrote (%s): it committed evidence during the push, and the push left that commit behind. Your commits now carry the evidence (pre-commit hook).\n", initx.DisplayHookPath(root))
		if err != nil {
			fmt.Fprintln(w, "shale: pre-commit hook:", err, "— run `shale init`")
		}
	}
	return true
}
