package hyperliquid

import "testing"

// TestRequestWeights pins the per-request Cost weights to the published
// table (retrieved 2026-10-31):
// https://hyperliquid.gitbook.io/hyperliquid-docs/for-developers/api/rate-limits-and-user-limits
func TestRequestWeights(t *testing.T) {
	weights := map[string]int{
		// weight 2 per the docs
		"l2Book":                 2,
		"allMids":                2,
		"clearinghouseState":     2,
		"orderStatus":            2,
		"spotClearinghouseState": 2,
		"exchangeStatus":         2,
		// weight 60 per the docs
		"userRole": 60,
		// all other documented info requests weigh 20
		"metaAndAssetCtxs":     20,
		"spotMetaAndAssetCtxs": 20,
		"frontendOpenOrders":   20,
		"candleSnapshot":       20,
	}
	for infoType, want := range weights {
		if got := infoWeight(infoType); got != want {
			t.Errorf("infoWeight(%q) = %d, want %d", infoType, got, want)
		}
	}

	// exchange actions: 1 + floor(batch_length/40)
	if got := exchangeWeight(OrderAction{Orders: make([]OrderWire, 79)}); got != 2 {
		t.Errorf("batched order of 79 weighs %d, want 2", got)
	}
	if got := exchangeWeight(OrderAction{Orders: make([]OrderWire, 40)}); got != 2 {
		t.Errorf("batched order of 40 weighs %d, want 2", got)
	}
	if got := exchangeWeight(CancelAction{Cancels: []CancelWire{}}); got != 1 {
		t.Errorf("unbatched cancel weighs %d, want 1", got)
	}
	if got := exchangeWeight(CancelByCloidAction{Cancels: make([]CancelByCloidWire, 3)}); got != 1 {
		t.Errorf("cancelByCloid of 3 weighs %d, want 1", got)
	}
	if got := exchangeWeight(TwapOrderAction{}); got != 1 {
		t.Errorf("twapOrder weighs %d, want 1", got)
	}
}
