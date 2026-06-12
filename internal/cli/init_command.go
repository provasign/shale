package cli

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/term"

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
func cmdInit(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(stderr)
	privacy := fs.String("privacy", store.PrivacyRedacted, "prompt privacy: full | redacted | hash-only")
	hooksOnly := fs.Bool("hooks-only", false, "only install the pre-push hook (fork contributor path)")
	global := fs.Bool("global", false, "also wire machine-wide hooks for agents installed on this machine")
	allowAgent := fs.Bool("allow-agent-commands", false, "auto-approve shale intent/done/note for Claude Code via the committed allowlist")
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

		// Auto-approving agent commands is a permission grant in a committed
		// file — explicit consent only: the flag, or a yes at a real terminal.
		// Never silently, never by default.
		consent := *allowAgent
		if !consent && isTerminal(stdin) {
			fmt.Fprint(stdout, "  ? Auto-approve shale evidence commands (shale intent/done/note) for Claude Code?\n"+
				"    Writes a permissions allowlist into the committed .claude/settings.json so your\n"+
				"    team's agents stop prompting for exactly these three commands. [y/N] ")
			consent = readYes(stdin)
		}
		if consent {
			if changed, err := initx.InstallClaudeAllowlist(filepath.Join(root, ".claude", "settings.json")); err != nil {
				fmt.Fprintln(stderr, "shale init: allowlist:", err)
				ok = false
			} else if changed {
				fmt.Fprintln(stdout, "  ✓ Claude auto-approve        (shale intent/done/note allowlisted in .claude/settings.json)")
			} else {
				fmt.Fprintln(stdout, "  ✓ Claude auto-approve        (already present)")
			}
		} else {
			fmt.Fprintln(stdout, "  – Claude auto-approve        (not configured — rerun with --allow-agent-commands to stop permission prompts)")
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
		if _, statErr := os.Stat(filepath.Join(root, ".git")); statErr != nil {
			fmt.Fprintln(stdout, "  ! Pre-push hook skipped      (not a git repo — run `git init`, then `shale init` again)")
		} else {
			fmt.Fprintf(stdout, "  ! Pre-push hook skipped      (existing hook at %s — add `shale finalize --auto-commit` to it, or evidence will never publish)\n", initx.DisplayHookPath(root))
		}
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

// isTerminal reports whether r is an interactive terminal — the only place
// a consent prompt may appear. Piped, scripted, and CI runs (including
// </dev/null, which a char-device check would misclassify) never see it.
func isTerminal(r io.Reader) bool {
	f, ok := r.(*os.File)
	return ok && term.IsTerminal(int(f.Fd()))
}

// readYes reads one line; only an explicit yes opts in. Default is No —
// this writes a permission grant into a committed file.
//
// The line ends at '\n' OR '\r': terminals left without ICRNL translation
// (seen in the wild with zsh/p10k setups) deliver Enter as a bare '\r', and
// waiting for '\n' there hangs the prompt with the user's keystrokes
// echoing as ^M.
func readYes(r io.Reader) bool {
	br := bufio.NewReader(r)
	var line []byte
	for len(line) < 64 {
		c, err := br.ReadByte()
		if err != nil || c == '\n' || c == '\r' {
			break
		}
		line = append(line, c)
	}
	switch strings.TrimSpace(strings.ToLower(string(line))) {
	case "y", "yes":
		return true
	}
	return false
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
