package pricing

import "testing"

func TestCostKnownModel(t *testing.T) {
	usd, ok := Cost("claude-fable-5", 32000, 15000)
	if !ok {
		t.Fatal("claude-fable-5 should be priced")
	}
	// 32000/1e6*5.00 + 15000/1e6*25.00 = 0.16 + 0.375 = 0.535 → 0.54
	if usd != 0.54 {
		t.Fatalf("cost = %v, want 0.54", usd)
	}
}

func TestCostDatedModelIDPrefixMatch(t *testing.T) {
	if _, ok := Cost("claude-sonnet-4-6-20250930", 1000, 1000); !ok {
		t.Fatal("dated model id should prefix-match")
	}
}

func TestCostUnknownModelOmitted(t *testing.T) {
	if _, ok := Cost("totally-new-model-9000", 1000, 1000); ok {
		t.Fatal("unknown model must not be priced (spec rule 8: never guess)")
	}
	if _, ok := Cost("", 1000, 1000); ok {
		t.Fatal("empty model must not be priced")
	}
}

func TestCostLongestPrefixWins(t *testing.T) {
	flash, _ := Cost("gemini-2.5-flash", 1e6, 0)
	pro, _ := Cost("gemini-2.5-pro", 1e6, 0)
	if flash >= pro {
		t.Fatalf("flash (%v) should be cheaper than pro (%v)", flash, pro)
	}
}
