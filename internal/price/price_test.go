package price

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Tuklus-Labs/aitop/internal/types"
)

func TestNormalizeStripsProviderOneMAndDate(t *testing.T) {
	for in, want := range map[string]string{
		"anthropic/claude-opus-5[1m]":     "claude-opus-5",
		"claude-haiku-4-5-20251001":       "claude-haiku-4-5",
		"  GPT-5.6-SOL ":                  "gpt-5.6-sol",
		"claude-fable-5":                  "claude-fable-5",
		"openai/gpt-daybreak-blue-latest": "gpt-daybreak-blue-latest",
	} {
		if got := Normalize(in); got != want {
			t.Fatalf("price-normalize violated: %q -> %q want %q", in, got, want)
		}
	}
}

func TestCostIsLifetimeUsageTimesListPrice(t *testing.T) {
	tb := Builtin()
	u := types.Usage{Input: 1_000_000, Output: 100_000, CacheRead: 10_000_000, CacheWrite: 500_000, Known: true}
	// fable-5: 10 + 5 + 10 + 10 = 35
	usd, src, ok := tb.Cost("claude-fable-5", u)
	if !ok || usd < 34.99 || usd > 35.01 || src != "table:builtin" {
		t.Fatalf("price-cost-math violated: usd=%v src=%q ok=%v want 35 table:builtin", usd, src, ok)
	}
	if _, _, ok := tb.Cost("claude-fable-5", types.Usage{Input: 5}); ok {
		t.Fatal("price-unknown-usage-is-absent violated: priced a session whose runtime reported no totals")
	}
	if _, _, ok := tb.Cost("some-model-nobody-priced", u); ok {
		t.Fatal("price-unknown-model-is-absent violated: invented a cost for an unpriced model")
	}
}

func TestLookupPrefixFallbackAndWindow(t *testing.T) {
	tb := Builtin()
	if _, _, ok := tb.Lookup("grok-4.6-fast-reasoning"); !ok {
		t.Fatal("price-lookup-longest-prefix violated: grok-4.6-fast-reasoning should fall back to grok-4.6")
	}
	if _, _, ok := tb.Lookup("grok"); ok {
		t.Fatal("price-lookup-no-short-prefix violated: bare 'grok' matched something")
	}
	if w, ok := tb.Window("claude-opus-5"); !ok || w != 1_000_000 {
		t.Fatalf("price-window-4-6-and-later-is-1m violated: %d %v", w, ok)
	}
	if w, ok := tb.Window("claude-haiku-4-5"); !ok || w != 200_000 {
		t.Fatalf("price-window-4-5-is-200k violated: %d %v", w, ok)
	}
	if w, ok := tb.Window("claude-opus-5[1m]"); !ok || w != 1_000_000 {
		t.Fatalf("price-window-1m-marker violated: %d %v", w, ok)
	}
	if _, ok := tb.Window("gpt-5.6-sol"); ok {
		t.Fatal("price-window-absent-when-table-has-none violated: openai rows carry no window")
	}
}

func TestUserFileOverridesAndAdds(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "prices.json")
	if err := os.WriteFile(p, []byte(`{"claude-fable-5":{"in":1,"out":2,"cache_read":0.1,"cache_write":2},"house-model":{"in":0,"out":0,"window":32768}}`), 0644); err != nil {
		t.Fatal(err)
	}
	tb := Builtin()
	if err := tb.LoadUser(p); err != nil {
		t.Fatal(err)
	}
	usd, src, ok := tb.Cost("claude-fable-5", types.Usage{Input: 1_000_000, Known: true})
	if !ok || usd != 1 || src != "table:user" {
		t.Fatalf("price-user-file-overrides-builtin violated: usd=%v src=%q", usd, src)
	}
	if w, ok := tb.Window("house-model"); !ok || w != 32768 {
		t.Fatalf("price-user-file-adds-models violated: %d %v", w, ok)
	}
	if err := tb.LoadUser(filepath.Join(dir, "missing.json")); err != nil {
		t.Fatalf("price-missing-user-file-is-fine violated: %v", err)
	}
	os.WriteFile(p, []byte(`{not json`), 0644)
	if err := Builtin().LoadUser(p); err == nil {
		t.Fatal("price-malformed-user-file-errors violated")
	}
}

func TestApplyNeverOverwritesRuntimeValues(t *testing.T) {
	tb := Builtin()
	known := 0.42
	win := int64(828400)
	ovs := []types.Overlay{
		{Model: "gpt-5.6-sol", Usage: types.Usage{Input: 1_000_000, Known: true}, CostUSD: &known, ContextWindow: &win},
		{Model: "claude-opus-5", Usage: types.Usage{Input: 1_000_000, Known: true}},
		{Model: "grok-4.6"}, // no totals: window only
	}
	tb.Apply(ovs)
	if *ovs[0].CostUSD != 0.42 || ovs[0].CostSource != "" || *ovs[0].ContextWindow != 828400 {
		t.Fatalf("price-apply-keeps-runtime-values violated: %+v", ovs[0])
	}
	if ovs[1].CostUSD == nil || *ovs[1].CostUSD != 5 || ovs[1].CostSource != "table:builtin" || ovs[1].WindowSource != "table" {
		t.Fatalf("price-apply-stamps-estimates violated: cost=%v src=%q winsrc=%q", ovs[1].CostUSD, ovs[1].CostSource, ovs[1].WindowSource)
	}
	if ovs[2].CostUSD != nil || ovs[2].ContextWindow == nil || *ovs[2].ContextWindow != 500_000 {
		t.Fatalf("price-apply-window-without-usage violated: %+v", ovs[2])
	}
}
