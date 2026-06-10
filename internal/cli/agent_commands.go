package cli

import (
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

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
	sessionID := store.CurrentSession(root)
	if sessionID == "" {
		// CLI-only session (no hook adapter ran): open one ourselves.
		sessionID = store.NewULID(time.Now())
		if err := store.SetCurrentSession(root, sessionID); err != nil {
			fmt.Fprintln(stderr, "shale intent:", err)
			return 1
		}
	}
	ev := store.Event{Kind: store.KindIntent, At: nowUTC(), Title: title, Body: *body}
	if err := store.AppendEvent(root, sessionID, ev); err != nil {
		fmt.Fprintln(stderr, "shale intent:", err)
		return 1
	}
	fmt.Fprintf(stdout, "✓ intent recorded (session %s)\n", short(sessionID))
	return 0
}

// cmdDone is the agent-called completion report — the semantic act, distinct
// from the mechanical `shale finalize` (pre-push).
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
	sessionID := store.CurrentSession(root)
	if sessionID == "" {
		// done without intent: still record it — absence of intent will be
		// explicit on the card, but the completion evidence is real.
		sessionID = store.NewULID(time.Now())
		if err := store.SetCurrentSession(root, sessionID); err != nil {
			fmt.Fprintln(stderr, "shale done:", err)
			return 1
		}
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
	fmt.Fprintf(stdout, "✓ completion recorded (session %s) — finalize runs on push\n", short(sessionID))
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
	sessionID := store.CurrentSession(root)
	if sessionID == "" {
		sessionID = store.NewULID(time.Now())
		if err := store.SetCurrentSession(root, sessionID); err != nil {
			fmt.Fprintln(stderr, "shale note:", err)
			return 1
		}
	}
	if err := store.AppendEvent(root, sessionID, store.Event{Kind: store.KindNote, At: nowUTC(), Text: text}); err != nil {
		fmt.Fprintln(stderr, "shale note:", err)
		return 1
	}
	fmt.Fprintln(stdout, "✓ note recorded")
	return 0
}

func short(id string) string {
	if len(id) > 6 {
		return id[:6]
	}
	return id
}
