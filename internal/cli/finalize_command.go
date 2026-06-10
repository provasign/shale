package cli

import (
	"flag"
	"fmt"
	"io"

	"github.com/provasign/shale/internal/fold"
	"github.com/provasign/shale/internal/gitx"
	"github.com/provasign/shale/internal/initx"
)

// cmdFinalize folds session events into committed shale YAML. Installed as
// the pre-push hook with --auto-commit; safe to run manually. Fail-open:
// finalize problems must never block a push (the hook script also exits 0).
func cmdFinalize(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("finalize", flag.ContinueOnError)
	fs.SetOutput(stderr)
	autoCommit := fs.Bool("auto-commit", false, "commit the written shale files (ADR D3)")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	root := repoRoot()
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
	if *autoCommit {
		if err := gitx.AutoCommit(root, res.Written, "chore(shale): session evidence"); err != nil {
			fmt.Fprintln(stderr, "shale finalize: auto-commit:", err)
			return 1
		}
		fmt.Fprintln(stdout, "shale: evidence committed")
	}
	return 0
}
