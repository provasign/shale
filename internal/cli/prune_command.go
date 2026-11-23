package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/provasign/shale/internal/store"
)

// cmdPrune deletes committed evidence older than a retention period.
//
// The PR card reads only the .shale/*.yaml files a pull request adds or
// changes, so evidence from merged work is history, not input: deleting it
// never changes a card. Rewriting it instead would, because the card flags
// evidence edited after its introducing commit. Files are dated by their
// own finalized_at (created_at when unfinalized); a file that does not
// parse is kept. Deletions are left in the working tree to commit.
func cmdPrune(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("prune", flag.ContinueOnError)
	fs.SetOutput(stderr)
	olderThan := fs.String("older-than", "", "delete evidence finalized more than this many days ago (e.g. 30 or 30d)")
	dryRun := fs.Bool("dry-run", false, "list what would be deleted; delete nothing")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *olderThan == "" {
		fmt.Fprintln(stderr, "shale prune: --older-than <days> is required")
		return 2
	}
	days, err := strconv.Atoi(strings.TrimSuffix(*olderThan, "d"))
	if err != nil || days < 1 {
		fmt.Fprintf(stderr, "shale prune: --older-than %q: want a positive number of days\n", *olderThan)
		return 2
	}
	cutoff := nowUTC().Add(-time.Duration(days) * 24 * time.Hour)
	root := repoRoot()
	dir := filepath.Join(root, ".shale")
	matches, _ := filepath.Glob(filepath.Join(dir, "*.yaml"))
	sort.Strings(matches)
	var old []string
	kept, unreadable := 0, 0
	for _, p := range matches {
		if filepath.Base(p) == "config.yaml" {
			continue
		}
		s, err := store.Load(p)
		if err != nil {
			unreadable++
			continue
		}
		when := s.FinalizedAt
		if when.IsZero() {
			when = s.CreatedAt
		}
		if when.IsZero() || !when.Before(cutoff) {
			kept++
			continue
		}
		old = append(old, p)
	}
	verb := "deleted"
	if *dryRun {
		verb = "would delete"
	}
	for _, p := range old {
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel) // git's form on every platform
		if !*dryRun {
			if err := os.Remove(p); err != nil {
				fmt.Fprintf(stderr, "shale prune: %s: %v\n", rel, err)
				return 1
			}
		}
		fmt.Fprintf(stdout, "%s %s\n", verb, rel)
	}
	fmt.Fprintf(stdout, "shale prune: %s %d evidence file(s) older than %d days (before %s); kept %d",
		verb, len(old), days, cutoff.Format("2006-01-02"), kept)
	if unreadable > 0 {
		fmt.Fprintf(stdout, "; kept %d that do not parse", unreadable)
	}
	fmt.Fprintln(stdout)
	if !*dryRun && len(old) > 0 {
		fmt.Fprintln(stdout, "commit the deletions: git add -A .shale && git commit -m \"shale: prune evidence older than "+strconv.Itoa(days)+" days\"")
	}
	return 0
}
