// Package pricing computes cost_usd from token counts. Spec rule 8: Shale
// owns the arithmetic, callers pass tokens, and the table version is recorded
// in the shale file so old costs stay reproducible after price changes.
package pricing

import "strings"

// Version identifies the pricing table snapshot recorded in shale files.
const Version = "2026-06-09"

type rate struct {
	inPerMTok  float64 // USD per million input tokens
	outPerMTok float64 // USD per million output tokens
}

// Prefix-matched so dated model IDs (claude-sonnet-4-6-20250930) resolve.
var table = map[string]rate{
	"claude-fable-5":   {5.00, 25.00},
	"claude-opus-4":    {15.00, 75.00},
	"claude-sonnet-4":  {3.00, 15.00},
	"claude-haiku-4":   {1.00, 5.00},
	"gpt-5":            {1.25, 10.00},
	"gpt-4.1":          {2.00, 8.00},
	"o3":               {2.00, 8.00},
	"gemini-2.5-pro":   {1.25, 10.00},
	"gemini-2.5-flash": {0.30, 2.50},
}

// Cost returns the USD cost for the model and token counts, rounded to
// cents. ok is false for unknown models — the caller must then omit
// cost_usd entirely (never guess).
func Cost(model string, tokensIn, tokensOut int) (usd float64, ok bool) {
	model = strings.ToLower(strings.TrimSpace(model))
	if model == "" {
		return 0, false
	}
	var best string
	for prefix := range table {
		if strings.HasPrefix(model, prefix) && len(prefix) > len(best) {
			best = prefix
		}
	}
	if best == "" {
		return 0, false
	}
	r := table[best]
	raw := float64(tokensIn)/1e6*r.inPerMTok + float64(tokensOut)/1e6*r.outPerMTok
	return float64(int(raw*100+0.5)) / 100, true
}
