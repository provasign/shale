package redact

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// A project can name words of its own that must never reach committed
// evidence: a client, a confidential codename, a project it studied. They are
// not secret-shaped, so the vendored rules cannot catch them. Each list holds
// one term per line; blank lines and lines starting with # are skipped. A
// term matches ignoring case, as a whole word where it starts or ends with a
// letter or digit, and is replaced by [REDACTED:term]: the placeholder never
// says which term it was.
//
// The lists are read from, in order:
//   - .shale/redact.txt, committed: terms the whole team keeps out of evidence
//     and may publish (a public repository's committed list is public);
//   - .shale/local/redact.txt, gitignored: terms private to this machine;
//   - SHALE_REDACT_FILE, one path or several separated like PATH: a list kept
//     anywhere, such as a private repository.
//
// A list that is named but cannot be read is an error, so evidence is never
// written without the terms it was meant to hide. A missing file is skipped.

// TermsEnv names extra term lists, separated like PATH.
const TermsEnv = "SHALE_REDACT_FILE"

// TermFiles lists the term files for the repository at root: those present,
// and every file SHALE_REDACT_FILE names, present or not.
func TermFiles(root string) []string {
	var out []string
	for _, p := range []string{
		filepath.Join(root, ".shale", "redact.txt"),
		filepath.Join(root, ".shale", "local", "redact.txt"),
	} {
		if _, err := os.Stat(p); err == nil {
			out = append(out, p)
		}
	}
	for _, p := range filepath.SplitList(os.Getenv(TermsEnv)) {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// LoadTerms reads the terms in files. Every file must be readable.
func LoadTerms(files ...string) ([]string, error) {
	seen := map[string]bool{}
	var terms []string
	for _, f := range files {
		fh, err := os.Open(f)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil, fmt.Errorf("redaction terms %s: the file is named but missing; nothing is written without it", f)
			}
			return nil, fmt.Errorf("redaction terms %s: %w", f, err)
		}
		sc := bufio.NewScanner(fh)
		for sc.Scan() {
			t := strings.TrimSpace(sc.Text())
			if t == "" || strings.HasPrefix(t, "#") || seen[strings.ToLower(t)] {
				continue
			}
			seen[strings.ToLower(t)] = true
			terms = append(terms, t)
		}
		err = sc.Err()
		fh.Close()
		if err != nil {
			return nil, fmt.Errorf("redaction terms %s: %w", f, err)
		}
	}
	return terms, nil
}

// ForRepo returns the default engine plus the terms listed for the
// repository at root.
func ForRepo(root string) (*Engine, error) {
	terms, err := LoadTerms(TermFiles(root)...)
	if err != nil {
		return nil, err
	}
	return New().WithTerms(terms), nil
}

// WithTerms adds a rule that masks every term in terms.
func (e *Engine) WithTerms(terms []string) *Engine {
	if len(terms) == 0 {
		return e
	}
	// Longest first, so a term is not cut short by a shorter one it contains.
	sorted := append([]string(nil), terms...)
	sort.Slice(sorted, func(i, j int) bool { return len(sorted[i]) > len(sorted[j]) })
	alts := make([]string, len(sorted))
	for i, t := range sorted {
		alts[i] = wordBounded(t)
	}
	re := regexp.MustCompile(`(?i)(?:` + strings.Join(alts, "|") + `)`)
	return &Engine{rules: append(append([]rule(nil), e.rules...), rule{name: "term", re: re})}
}

// wordBounded quotes t, bounded as a whole word on each side that starts or
// ends with a letter, digit or underscore.
func wordBounded(t string) string {
	q := regexp.QuoteMeta(t)
	r := []rune(t)
	if isWord(r[0]) {
		q = `\b` + q
	}
	if isWord(r[len(r)-1]) {
		q += `\b`
	}
	return q
}

func isWord(r rune) bool {
	return r == '_' || r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r))
}
