package render

import (
	"strings"
	"testing"

	"github.com/provasign/shale/internal/store"
)

const steleCard = `{
  "stele_card": 1,
  "change": "summer",
  "format": "shale",
  "status": "approved",
  "checked_against": 2,
  "requirements": [
    {"id": "intent/s2", "title": "Summer reading program", "change": "added", "reading": ["precondition Member.borrow when (self.booksOnLoan < 10)"]},
    {"id": "intent/s1", "title": "Limit loans", "change": "retired"}
  ],
  "decisions": [{"question": "\"Limit loans\" and \"Summer reading program\" cannot both hold. Which is right?", "answer": "One is an exception to the other: restate them as one request", "by": "owner", "at": "2026-11-02"}],
  "open_questions": 0,
  "approved_by": "owner",
  "approved_at": "2026-11-02",
  "touched": [{"statement": "intent/s2", "title": "Summer reading program", "evidence": ["Borrow (lib.go:7) is the step Member.borrow"]}],
  "updated": "2026-11-02T03:57:39Z",
  "future_field": true
}`

func TestCardRendersSteleRequirements(t *testing.T) {
	c, err := ParseSteleCard([]byte(steleCard))
	if err != nil {
		t.Fatal(err)
	}
	s := sampleShale("a1b2c3d4e5")
	s.Files = []store.FileTouch{{Path: "lib.go", Ops: []string{"edit"}, Via: store.ViaHook}}
	got := Card(Input{Shales: []*store.Shale{s}, Stele: []*SteleCard{c}, PRFiles: []ChangedFile{
		{Path: "lib.go", Status: "modified"},
		{Path: ".stele/cards/summer.json", Status: "added"},
		{Path: ".stele/registry/change-summer.yaml", Status: "added"},
	}})
	for _, must := range []string{
		"### Requirements · Stele",
		"> ✅ **Approved by owner on 2026-11-02** · checked against 2 approved requirements",
		"| Summer reading program | added |",
		"| Limit loans | retired |",
		"How Stele reads them",
		"<code>precondition Member.borrow when (self.booksOnLoan &lt; 10)</code>",
		"→ *One is an exception to the other: restate them as one request* (owner, 2026-11-02)",
		"**Code touching approved requirements** (as of 2026-11-02 03:57 UTC)",
		"- Summer reading program — Borrow (lib.go:7) is the step Member.borrow",
		"did not check the code against them",
		"all with session evidence",
	} {
		if !strings.Contains(got, must) {
			t.Errorf("card missing %q:\n%s", must, got)
		}
	}
	if strings.Contains(got, ".stele/registry") {
		t.Errorf("Stele's bookkeeping must not appear in the file table when its section accounts for it:\n%s", got)
	}
	// Without a Stele section, a changed .stele/ file is an ordinary file.
	if got := Card(Input{Shales: []*store.Shale{s}, PRFiles: []ChangedFile{{Path: ".stele/registry/x.yaml", Status: "added"}}}); !strings.Contains(got, ".stele/registry/x.yaml") {
		t.Errorf("without a Stele card, .stele/ files stay visible:\n%s", got)
	}
}

func TestSteleCardStatuses(t *testing.T) {
	for status, want := range map[string]string{
		"questions": "⏳ **Waiting for the owner**: 2 questions open",
		"ready":     "⏳ **Waiting for the owner's approval**",
		"clean":     "✅ **Nothing to decide**",
		"broken":    "⚠️ **Stele could not check this change**",
		"weird":     "⚠️ **Unknown Stele status** `weird`",
	} {
		var b strings.Builder
		writeStele(&b, []*SteleCard{{Version: 1, Status: status, Open: 2}})
		if !strings.Contains(b.String(), want) {
			t.Errorf("%s: got %q, want %q", status, b.String(), want)
		}
	}
}

func TestSteleCardIsData(t *testing.T) {
	if _, err := ParseSteleCard([]byte(`{"stele_card": 2, "status": "approved"}`)); err == nil {
		t.Error("a newer major version must be refused")
	}
	if _, err := ParseSteleCard([]byte(`not json`)); err == nil {
		t.Error("garbage must be refused")
	}
	hostile := &SteleCard{Version: 1, Status: "approved", ApprovedBy: "@everyone <img src=x>",
		Requirements: []struct {
			Title   string   `json:"title"`
			Change  string   `json:"change"`
			Reading []string `json:"reading"`
		}{{Title: "a | b [x](https://evil) #1", Change: "added", Reading: []string{"`break` out"}}}}
	var b strings.Builder
	writeStele(&b, []*SteleCard{hostile})
	out := b.String()
	if !strings.Contains(out, `| a \| b`) {
		t.Errorf("a pipe in a title must not split the table row:\n%s", out)
	}
	for _, bad := range []string{"@everyone", "<img", "](https://evil)", "#1"} {
		if strings.Contains(out, bad) {
			t.Errorf("hostile text %q survived:\n%s", bad, out)
		}
	}
}
