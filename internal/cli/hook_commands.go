package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/provasign/shale/internal/fold"
	"github.com/provasign/shale/internal/gitx"
	"github.com/provasign/shale/internal/initx"
	"github.com/provasign/shale/internal/store"
)

// cmdPrecommit is the git pre-commit hook: it records the tasks closed since
// the last commit (shale done, or the agent's session ended) and stages
// their evidence, so the commit being made carries it alongside the code. A
// task still in progress stays open, to be recorded with a later commit. It
// never blocks the commit.
func cmdPrecommit(args []string, stdout, stderr io.Writer) int {
	root := repoRoot()
	if _, err := os.Stat(filepath.Join(root, ".shale", "schema-version")); err != nil {
		return 0 // not opted in
	}
	res, err := fold.Run(fold.Options{RepoRoot: root, Privacy: initx.LoadConfig(root).Privacy, ClosedOnly: true})
	if err != nil {
		fmt.Fprintln(stderr, "shale precommit:", err, "(commit continues)")
		return 0
	}
	if len(res.Written) == 0 {
		return 0
	}
	if err := gitx.Stage(root, res.Written); err != nil {
		fmt.Fprintln(stderr, "shale precommit: staging the evidence:", err, "(commit continues)")
		return 0
	}
	fmt.Fprintf(stderr, "shale: this commit carries the evidence of %d task(s):\n", len(res.Written))
	for _, p := range res.Written {
		fmt.Fprintf(stderr, "  %s  %s\n", p, describeRecord(filepath.Join(root, p)))
	}
	return 0
}

// cmdPrepush is the git pre-push hook. It never commits, so nothing is
// created while the push is in flight, and it never blocks: it says which
// sessions' evidence this push does not carry yet.
func cmdPrepush(args []string, stdout, stderr io.Writer) int {
	root := repoRoot()
	ids, err := store.ListSessions(root)
	if err != nil {
		return 0
	}
	for _, id := range ids {
		events, err := store.ReadEvents(store.SessionPath(root, id))
		if err != nil {
			continue
		}
		title, tool := "", "cli"
		closed := false
		for _, ev := range events {
			switch ev.Kind {
			case store.KindIntent:
				title, closed = ev.Title, false
			case store.KindCompletion, store.KindSessionClose:
				closed = true
			case store.KindSessionMeta:
				if ev.Tool != "" {
					tool = ev.Tool
				}
			}
		}
		switch {
		case title == "" && !closed:
			continue // ambient capture: nothing to publish
		case closed:
			fmt.Fprintf(stderr, "shale: %s · %q is done but not committed: it goes with your next commit (or run `shale finalize --auto-commit`)\n", tool, title)
		default:
			fmt.Fprintf(stderr, "shale: %s · %q is still open: its evidence goes with the first commit after `shale done`\n", tool, title)
		}
	}
	return 0
}
