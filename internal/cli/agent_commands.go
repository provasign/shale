package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/provasign/shale/internal/fold"
	"github.com/provasign/shale/internal/gitx"
	"github.com/provasign/shale/internal/initx"
	"github.com/provasign/shale/internal/store"
)

// parseInterspersed parses flags wherever they appear among positional
// arguments. The documented call shape is `shale intent "<title>" --body
// "..."`, but stdlib flag stops at the first positional, silently folding
// trailing flags into the title — agents follow the docs, so accept it.
func parseInterspersed(fs *flag.FlagSet, args []string) ([]string, error) {
	var positional []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		args = fs.Args()
		if len(args) == 0 {
			return positional, nil
		}
		positional = append(positional, args[0])
		args = args[1:]
	}
}

// cmdIntent is the agent-called intent declaration (ADR D4 tier 1). It must
// be fast (<50 ms) and its single-line ack is the only Shale text that
// should enter the agent's context.
func cmdIntent(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("intent", flag.ContinueOnError)
	fs.SetOutput(stderr)
	body := fs.String("body", "", "why, constraints, approach (2–5 sentences)")
	positional, err := parseInterspersed(fs, args)
	if err != nil {
		return 1
	}
	title := strings.TrimSpace(strings.Join(positional, " "))
	if title == "" {
		fmt.Fprintln(stderr, `usage: shale intent "<title>" [--body "..."]`)
		return 1
	}

	root := repoRoot()
	sessionID, err := agentSession(root)
	if err != nil {
		fmt.Fprintln(stderr, "shale intent:", err)
		return 1
	}
	ev := store.Event{Kind: store.KindIntent, At: nowUTC(), Title: title, Body: *body}
	if err := store.AppendEvent(root, sessionID, ev); err != nil {
		fmt.Fprintln(stderr, "shale intent:", err)
		return 1
	}
	fmt.Fprintf(stdout, "✓ intent recorded (session %s)\n", short(sessionID))
	return 0
}

// cmdDone is the agent-called completion report — the semantic act. In repos
// that opted in (a .shale/ scaffold exists) it also finalizes and commits the
// evidence on the spot: the commit then exists BEFORE the user pushes, so it
// rides their normal push with no pre-push hook involved and no
// does-the-hook-commit-make-the-push ambiguity. The pre-push hook remains the
// mechanical safety net for sessions that never got a done.
func cmdDone(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("done", flag.ContinueOnError)
	fs.SetOutput(stderr)
	note := fs.String("note", "", "what you did, any deviations")
	tokensIn := fs.Int("tokens-in", 0, "prompt tokens for the session")
	tokensOut := fs.Int("tokens-out", 0, "completion tokens for the session")
	model := fs.String("model", "", "model id, as reported by the agent")
	iterations := fs.Int("iterations", 0, "prompt-response cycles")
	if _, err := parseInterspersed(fs, args); err != nil {
		return 1
	}

	root := repoRoot()
	sessionID, err := agentSession(root)
	if err != nil {
		fmt.Fprintln(stderr, "shale done:", err)
		return 1
	}
	ev := store.Event{
		Kind: store.KindCompletion, At: nowUTC(),
		Note: *note, TokensIn: *tokensIn, TokensOut: *tokensOut,
		Model: *model, Iterations: *iterations,
	}
	if err := store.AppendEvent(root, sessionID, ev); err != nil {
		fmt.Fprintln(stderr, "shale done:", err)
		return 1
	}
	// Finalize-on-done, gated on the committed scaffold marker (not the bare
	// .shale/ dir — our own AppendEvent above just created that). The
	// evidence goes into the user's own commit: it is staged, and the code
	// commit that follows carries it. Only when the code is committed
	// already, so no commit of the user's is coming, does it get a commit of
	// its own, still before the push. Fail-open: any problem falls back to
	// the pre-commit hook and must not fail the done.
	if _, statErr := os.Stat(filepath.Join(root, ".shale", "schema-version")); statErr == nil {
		// Only this agent's session: the others open in the checkout are
		// their agents' to finalize, and committing them here published
		// another agent's work (2026-11-03).
		res, ferr := fold.Run(fold.Options{RepoRoot: root, Privacy: initx.LoadConfig(root).Privacy, Sessions: []string{sessionID}, ClosedOnly: true})
		if ferr == nil && len(res.Written) > 0 {
			if gitx.OnlyEvidencePending(root) {
				if committed, _ := gitx.AutoCommit(root, res.Written, "chore(shale): session evidence"); committed {
					fmt.Fprintf(stdout, "✓ completion recorded (session %s) — the code was already committed, so the evidence got its own commit; it publishes with your push\n", short(sessionID))
					return 0
				}
			} else if gitx.Stage(root, res.Written) == nil {
				fmt.Fprintf(stdout, "✓ completion recorded (session %s) — evidence staged; commit it with your code\n", short(sessionID))
				return 0
			}
		}
	}
	fmt.Fprintf(stdout, "✓ completion recorded (session %s) — the pre-commit hook adds its evidence to your next commit\n", short(sessionID))
	// The one silent-failure mode worth breaking the single-line ack for:
	// evidence wasn't committed here, and a pre-push hook that never runs
	// finalize means it will publish nowhere. `done` is the last moment an
	// agent can relay that to the user.
	if warn := initx.FinalizeHookWarning(root); warn != "" {
		fmt.Fprintln(stderr, warn)
	}
	return 0
}

// cmdNote is the manual annotation escape hatch.
func cmdNote(args []string, stdout, stderr io.Writer) int {
	text := strings.TrimSpace(strings.Join(args, " "))
	if text == "" {
		fmt.Fprintln(stderr, `usage: shale note "<text>"`)
		return 1
	}
	root := repoRoot()
	sessionID, err := agentSession(root)
	if err != nil {
		fmt.Fprintln(stderr, "shale note:", err)
		return 1
	}
	if err := store.AppendEvent(root, sessionID, store.Event{Kind: store.KindNote, At: nowUTC(), Text: text}); err != nil {
		fmt.Fprintln(stderr, "shale note:", err)
		return 1
	}
	fmt.Fprintln(stdout, "✓ note recorded")
	return 0
}

// sessionEnv names, per coding agent, the variable that carries its session
// id into the commands it runs: the same id its hooks report, so a CLI call
// lands in the session its hooks capture.
var sessionEnv = []struct{ name, tool string }{
	{"CLAUDE_CODE_SESSION_ID", "claude-code"},
	{"CODEX_THREAD_ID", "codex"},
}

// agentSession is the session a CLI call (intent, done, note) belongs to.
// The calling agent's own session id comes first: several agents can work
// in one repository at once, and .shale/local/current names only the one
// whose hook ran last. An agent that exposes no id falls back to that
// pointer, and with no pointer either a session is opened here.
func agentSession(root string) (string, error) {
	for _, e := range sessionEnv {
		id := strings.TrimSpace(os.Getenv(e.name))
		if id == "" || strings.ContainsAny(id, `/\`) || strings.Contains(id, "..") {
			continue
		}
		if _, err := os.Stat(store.SessionPath(root, id)); os.IsNotExist(err) {
			// The agent's hooks have not run here (not installed, or not
			// trusted yet): say which tool the session is.
			if err := store.AppendEvent(root, id, store.Event{Kind: store.KindSessionMeta, At: nowUTC(), Tool: e.tool}); err != nil {
				return "", err
			}
		}
		return id, nil
	}
	if id := store.CurrentSession(root); id != "" {
		return id, nil
	}
	// CLI-only session (no hook adapter ran): open one ourselves.
	id := store.NewULID(time.Now())
	return id, store.SetCurrentSession(root, id)
}

func short(id string) string {
	if len(id) > 6 {
		return id[:6]
	}
	return id
}
