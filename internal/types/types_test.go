package types

import "testing"

func TestZeroOverlayCostIsAbsentNotZero(t *testing.T) {
	var o Overlay
	if o.CostUSD != nil {
		t.Fatalf("unknown-cost-is-absent violated: zero Overlay CostUSD=%v (missing overlay must not become 0)", o.CostUSD)
	}
	if o.TokensUsed != nil || o.ContextWindow != nil {
		t.Fatalf("absent-token-fields-are-nil violated: TokensUsed=%v ContextWindow=%v", o.TokensUsed, o.ContextWindow)
	}
	if o.ProvenName != "" {
		t.Fatalf("unproven-name-is-empty violated: ProvenName=%q", o.ProvenName)
	}
}

func TestOverlayControlFieldsZeroMeanAbsent(t *testing.T) {
	var o Overlay
	if o.ForkOf != "" || o.CapsuleID != "" || o.Worktree != "" || o.Kind != "" || o.TokPerSec != nil || o.SlotIndex != nil || o.Dark {
		t.Fatalf("control-fields-zero-mean-absent violated: %+v", o)
	}
}
