package main

import (
	"testing"
	"time"

	m "github.com/plusev-terminal/go-plugin-common/meta"
)

// TestManifestRateLimits pins the manifest rate-limit declarations to the
// published Hyperliquid budget so the numbers can't silently drift.
// Source (retrieved 2026-10-31):
// https://hyperliquid.gitbook.io/hyperliquid-docs/for-developers/api/rate-limits-and-user-limits
func TestManifestRateLimits(t *testing.T) {
	targets := (&HyperliquidPlugin{}).GetMeta().Resources.AllowedNetworkTargets

	byPattern := map[string]*m.NetworkTargetRule{}
	for i := range targets {
		byPattern[targets[i].Pattern] = &targets[i]
	}

	for _, pattern := range []string{
		"https://api.hyperliquid.xyz/*",
		"https://api.hyperliquid-testnet.xyz/*",
	} {
		t.Run(pattern, func(t *testing.T) {
			target := byPattern[pattern]
			if target == nil {
				t.Fatalf("missing network target %s", pattern)
			}
			if len(target.Limits) != 1 {
				t.Fatalf("expected exactly 1 limit rule, got %d", len(target.Limits))
			}
			rule := target.Limits[0]
			// 1200 weight per minute per IP = 20 weight/s; no smaller bucket
			// is published, so burst stays 0 (= full window, 1200).
			if rule.ID != "weight" || rule.Limit != 1200 || rule.WindowMs != time.Minute.Milliseconds() {
				t.Errorf("weight rule drifted: %+v", rule)
			}
			if rule.Burst != 0 || rule.Scope != "" || !rule.CostWeighted || len(rule.Paths) != 0 {
				t.Errorf("weight rule shape drifted (want default IP scope, weighted, all paths): %+v", rule)
			}
			if target.TradingReserve == nil || *target.TradingReserve != 0.3 {
				t.Errorf("expected trading reserve 0.3, got %+v", target.TradingReserve)
			}
		})
	}

	for _, pattern := range []string{
		"wss://api.hyperliquid.xyz/*",
		"wss://api.hyperliquid-testnet.xyz/*",
	} {
		if target := byPattern[pattern]; target == nil || len(target.Limits) != 0 {
			t.Errorf("websocket target %s must stay unlimited", pattern)
		}
	}

	// Manifest limits replace the legacy per-command limiter: explicit
	// empty-slice opt-out, keeping the export alive.
	if limits := (&HyperliquidPlugin{}).GetRateLimits(); len(limits) != 0 {
		t.Errorf("GetRateLimits must opt out with an empty slice, got %d entries", len(limits))
	}
}
