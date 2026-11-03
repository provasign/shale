package render

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// SteleCardDir is where Stele (provasign/stele) commits its part of a PR's
// evidence: one JSON file per checked change. Shale renders it as read,
// never running Stele: the card stays a pure function of committed files.
const SteleCardDir = ".stele/cards"

// steleCardVersion is the newest Stele card major version this build reads.
const steleCardVersion = 1

// SteleCard is Stele's account of what a change does to the project's
// approved requirements. Every string is attacker-controlled data.
type SteleCard struct {
	Version      int    `json:"stele_card"`
	Change       string `json:"change"`
	Format       string `json:"format"`
	Status       string `json:"status"` // approved | ready | questions | clean | broken | failed
	Checked      int    `json:"checked_against"`
	Requirements []struct {
		Title   string   `json:"title"`
		Change  string   `json:"change"` // added | modified | retired
		Reading []string `json:"reading"`
	} `json:"requirements"`
	Decisions []struct {
		Question string `json:"question"`
		Answer   string `json:"answer"`
		By       string `json:"by"`
		At       string `json:"at"`
	} `json:"decisions"`
	Open       int    `json:"open_questions"`
	ApprovedBy string `json:"approved_by"`
	ApprovedAt string `json:"approved_at"`
	Touched    []struct {
		Title    string   `json:"title"`
		Evidence []string `json:"evidence"`
	} `json:"touched"`
	Updated string `json:"updated"`
}

// ParseSteleCard reads one card, refusing a newer major version.
func ParseSteleCard(raw []byte) (*SteleCard, error) {
	var c SteleCard
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, err
	}
	if c.Version < 1 || c.Version > steleCardVersion {
		return nil, fmt.Errorf("stele card version %d: this shale reads up to %d", c.Version, steleCardVersion)
	}
	return &c, nil
}

// maxSteleRows caps each list in the section; the rest is counted.
const maxSteleRows = 12

func writeStele(b *strings.Builder, cards []*SteleCard) {
	if len(cards) == 0 {
		return
	}
	b.WriteString("\n### Requirements · Stele\n")
	for i, c := range cards {
		if i > 0 {
			b.WriteString("\n")
		}
		writeSteleStatus(b, c)
		if len(c.Requirements) > 0 {
			b.WriteString("\n| Requirement | Change |\n|---|---|\n")
			for k, r := range c.Requirements {
				if k == maxSteleRows {
					fmt.Fprintf(b, "| … %d more | |\n", len(c.Requirements)-k)
					break
				}
				fmt.Fprintf(b, "| %s | %s |\n", cell(r.Title), cell(r.Change))
			}
			var readings []string
			for _, r := range c.Requirements {
				for _, x := range r.Reading {
					// <code>, not a code span: GitHub shows the escaped
					// text of an HTML element as written, so ">" reads ">".
					readings = append(readings, fmt.Sprintf("- %s: <code>%s</code>", Sanitize(r.Title), Sanitize(x)))
				}
			}
			if len(readings) > 0 {
				b.WriteString("\n<details><summary>How Stele reads them</summary>\n\n")
				b.WriteString(strings.Join(readings[:min(len(readings), 3*maxSteleRows)], "\n") + "\n\n</details>\n")
			}
		}
		if len(c.Decisions) > 0 {
			b.WriteString("\n**Owner decisions**\n")
			for k, d := range c.Decisions {
				if k == maxSteleRows {
					fmt.Fprintf(b, "- … %d more\n", len(c.Decisions)-k)
					break
				}
				fmt.Fprintf(b, "- %s → *%s* (%s, %s)\n", Sanitize(d.Question), Sanitize(d.Answer), Sanitize(d.By), Sanitize(d.At))
			}
		}
		if len(c.Touched) > 0 {
			fmt.Fprintf(b, "\n**Code touching approved requirements**%s\n", asOf(c.Updated))
			for k, t := range c.Touched {
				if k == maxSteleRows {
					fmt.Fprintf(b, "- … %d more\n", len(c.Touched)-k)
					break
				}
				line := "- " + Sanitize(t.Title)
				if len(t.Evidence) > 0 {
					line += " — " + Sanitize(strings.Join(t.Evidence, "; "))
				}
				b.WriteString(line + "\n")
			}
			b.WriteString("\n*Stele matched code to the requirements by name; it did not check the code against them.*\n")
		}
	}
}

func writeSteleStatus(b *strings.Builder, c *SteleCard) {
	against := ""
	if c.Checked > 0 {
		against = fmt.Sprintf(" · checked against %d approved requirement%s", c.Checked, plural(c.Checked))
	}
	switch c.Status {
	case "approved":
		who := ""
		if c.ApprovedBy != "" {
			who = " by " + Sanitize(c.ApprovedBy)
		}
		if c.ApprovedAt != "" {
			who += " on " + Sanitize(c.ApprovedAt)
		}
		fmt.Fprintf(b, "> ✅ **Approved%s**%s\n", who, against)
	case "clean":
		fmt.Fprintf(b, "> ✅ **Nothing to decide**: no requirement changed%s\n", against)
	case "ready":
		fmt.Fprintf(b, "> ⏳ **Waiting for the owner's approval**: no conflicts%s\n", against)
	case "questions":
		fmt.Fprintf(b, "> ⏳ **Waiting for the owner**: %d question%s open%s\n", c.Open, plural(c.Open), against)
	case "broken":
		fmt.Fprintf(b, "> ⚠️ **Stele could not check this change**: its rules do not compile together\n")
	case "failed":
		fmt.Fprintf(b, "> ⚠️ **Stele could not check this change**: the model could not read its requirements; nothing was checked\n")
	case "undecided":
		fmt.Fprintf(b, "> ⚠️ **Stele could not decide part of this change**: the solver could not settle what a step must do; the owner can word that requirement more simply\n")
	default:
		fmt.Fprintf(b, "> ⚠️ **Unknown Stele status** %s\n", code(Sanitize(c.Status)))
	}
}

// cell is a table cell: sanitized, with pipes escaped.
func cell(s string) string {
	return strings.ReplaceAll(Sanitize(s), "|", "\\|")
}

func asOf(stamp string) string {
	t, err := time.Parse(time.RFC3339, stamp)
	if err != nil {
		return ""
	}
	return " (as of " + formatTimeUTC(t) + ")"
}
