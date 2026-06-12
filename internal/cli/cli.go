// Package cli wires the shale commands. Commands stay thin here; the logic
// lives in the internal packages where it is unit-tested.
package cli

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/provasign/shale/internal/gitx"
	"github.com/provasign/shale/internal/version"
)

const usage = `shale — agent PR evidence (https://github.com/provasign/shale)

Usage:
  shale init                     set up this repo (steering prompt, hooks, workflow)
  shale intent "<title>" [--body "..."]
                                 agent-called BEFORE the first file edit
  shale done [--note "..."] [--tokens-in N] [--tokens-out N]
             [--model M] [--iterations N]
                                 agent-called AFTER the work is complete
  shale capture <adapter>        hook entry point (reads hook JSON on stdin)
  shale finalize [--auto-commit] fold session events into committed shale YAML
  shale render --local | --pr N  render the card (terminal preview | post to PR)
  shale note "<text>"            manual annotation escape hatch
  shale doctor                   diagnose the setup
  shale uninstall [--repo]       remove Shale from this machine (--repo: also the committed files)
  shale version                  print version
`

// Run dispatches argv (without the program name) and returns the exit code.
func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 1
	}
	switch args[0] {
	case "init":
		return cmdInit(args[1:], stdin, stdout, stderr)
	case "intent":
		return cmdIntent(args[1:], stdout, stderr)
	case "done":
		return cmdDone(args[1:], stdout, stderr)
	case "capture":
		return cmdCapture(args[1:], stdin, stderr)
	case "finalize":
		return cmdFinalize(args[1:], stdout, stderr)
	case "render":
		return cmdRender(args[1:], stdout, stderr)
	case "note":
		return cmdNote(args[1:], stdout, stderr)
	case "doctor":
		return cmdDoctor(stdout)
	case "uninstall":
		return cmdUninstall(args[1:], stdout, stderr)
	case "version", "--version", "-v":
		fmt.Fprintln(stdout, "shale", version.Version)
		return 0
	case "help", "--help", "-h":
		fmt.Fprint(stdout, usage)
		return 0
	default:
		fmt.Fprintf(stderr, "shale: unknown command %q\n\n%s", args[0], usage)
		return 1
	}
}

// repoRoot resolves the repository root from the working directory, falling
// back to the directory itself so shale works in not-yet-git repos.
func repoRoot() string {
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	if root := gitx.Root(wd); root != "" {
		return root
	}
	return wd
}

func nowUTC() time.Time { return time.Now().UTC() }
