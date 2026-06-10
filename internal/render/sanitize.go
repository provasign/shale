package render

import "strings"

// Sanitize neutralizes shale-derived text before it enters the privileged
// card comment (workstream D6). Shale content is attacker-controlled input —
// a hostile fork PR can craft any text it wants in a shale file. We:
//   - HTML-escape, killing raw-HTML injection;
//   - break @-mentions and #-issue refs with a zero-width space, so the
//     card cannot ping people or link issues on the attacker's behalf;
//   - defang markdown link/image targets by breaking the "](" pair;
//   - strip control characters that could mangle the comment.
func Sanitize(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '&':
			b.WriteString("&amp;")
		case '@':
			b.WriteString("@\u200b")
		case '#':
			b.WriteString("#\u200b")
		case '\n', '\t':
			b.WriteRune(r)
		default:
			if r < 0x20 || r == 0x7f {
				continue
			}
			b.WriteRune(r)
		}
	}
	// Defang [text](target) links: the rendered card shows the text but the
	// target is severed.
	return strings.ReplaceAll(b.String(), "](", "]\u200b(")
}
