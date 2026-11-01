package main

import (
	"slices"
	"testing"
	"time"

	ex "github.com/plusev-terminal/go-plugin-common/v2/datasrc/exchange"
	acct "github.com/plusev-terminal/go-plugin-common/v2/datasrc/exchange/account"
	m "github.com/plusev-terminal/go-plugin-common/v2/meta"
	tt "github.com/plusev-terminal/go-plugin-common/v2/trading"
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

// TestOrderCapabilities pins the declared order surface to what
// hyperliquid/createOrders.go really implements, and proves the contract's
// Check enforces it: Hyperliquid serves spot+perp, always triggers on the
// mark price, has no trailing stops / GTD / close-position, and the only
// Extra keys consumed are the TWAP ones.
func TestOrderCapabilities(t *testing.T) {
	caps := (&HyperliquidPlugin{}).GetOrderCapabilities()
	assertSlice(t, "asset types", []tt.AssetType(caps.AssetTypes),
		[]tt.AssetType{tt.AssetTypeSpot, tt.AssetTypePerpetual})
	assertSlice(t, "order types", []acct.OrderType(caps.OrderTypes), []acct.OrderType{
		acct.OrderTypeLimit, acct.OrderTypeMarket,
		acct.OrderTypeStop, acct.OrderTypeStopLimit, acct.OrderTypeStopMarket,
		acct.OrderTypeTakeProfit, acct.OrderTypeTakeProfitLimit, acct.OrderTypeTakeProfitMarket,
	})
	assertSlice(t, "timeInForce", []acct.TimeInForce(caps.TimeInForce), []acct.TimeInForce{
		acct.TimeInForceGTC, acct.TimeInForceIOC, acct.TimeInForcePostOnly, acct.TimeInForceGTX,
	})
	assertSlice(t, "trigger sources", []acct.TriggerSource(caps.TriggerSources),
		[]acct.TriggerSource{acct.TriggerSourceMark})
	assertSlice(t, "extra keys", caps.ExtraKeys, []string{"twap", "minutes", "randomize"})

	if caps.DefaultTriggerSource != acct.TriggerSourceMark {
		t.Errorf("defaultTriggerSource = %q, want mark", caps.DefaultTriggerSource)
	}

	for _, flag := range []struct {
		name string
		got  bool
		want bool
	}{
		{"reduceOnly", caps.ReduceOnly, true},
		{"postOnly", caps.PostOnly, true},
		{"quoteQuantity", caps.QuoteQuantity, true},
		{"clientOrderId", caps.ClientOrderID, true},
		{"trailingAmount", caps.TrailingAmount, false},
		{"trailingPercent", caps.TrailingPercent, false},
		{"activationPrice", caps.ActivationPrice, false},
		{"closePosition", caps.ClosePosition, false},
		{"positionSide", caps.PositionSide, false},
		{"cancelGroupOnFailure", caps.CancelGroupOnFailure, false},
	} {
		if flag.got != flag.want {
			t.Errorf("%s = %v, want %v", flag.name, flag.got, flag.want)
		}
	}

	// The host's pre-dispatch gate must accept what the venue serves and
	// reject what it cannot honor.
	btc := tt.Market{Symbol: "BTC", AssetType: tt.AssetTypePerpetual}
	purr := tt.Market{Symbol: "PURR/USDC", AssetType: tt.AssetTypeSpot}

	if err := caps.Check(ex.CreateOrderParams{
		Market: btc, Side: acct.OrderSideSell, Type: acct.OrderTypeStop,
		Quantity: "1", TriggerPrice: "41000", TriggerSource: acct.TriggerSourceMark,
	}); err != nil {
		t.Errorf("mark-triggered stop on perp must pass, got: %v", err)
	}

	if err := caps.Check(ex.CreateOrderParams{
		Market: purr, Side: acct.OrderSideBuy, Type: acct.OrderTypeLimit,
		Quantity: "1", Price: "1.5", ClientOrderID: "0x1", ReduceOnly: false,
	}); err != nil {
		t.Errorf("spot limit must pass, got: %v", err)
	}

	for name, order := range map[string]ex.CreateOrderParams{
		"last trigger source": {
			Market: btc, Side: acct.OrderSideSell, Type: acct.OrderTypeStop,
			Quantity: "1", TriggerPrice: "1", TriggerSource: acct.TriggerSourceLast,
		},
		"trailing stop": {
			Market: btc, Side: acct.OrderSideSell, Type: acct.OrderTypeTrailingStop,
			Quantity: "1", TrailingAmount: "500",
		},
		"gtd expiry": {
			Market: btc, Side: acct.OrderSideBuy, Type: acct.OrderTypeLimit,
			Quantity: "1", Price: "1",
			TimeInForce: acct.TimeInForceGTD, ExpireTime: "9999999999999",
		},
		"close position": {
			Market: btc, Side: acct.OrderSideSell, Type: acct.OrderTypeMarket,
			Price: "1", ClosePosition: true,
		},
		"unknown extra key": {
			Market: btc, Side: acct.OrderSideBuy, Type: acct.OrderTypeLimit,
			Quantity: "1", Price: "1", Extra: map[string]any{"iceberg": true},
		},
		"unlisted asset type": {
			Market: tt.Market{Symbol: "BTC", AssetType: tt.AssetTypeFutures},
			Side:   acct.OrderSideBuy, Type: acct.OrderTypeLimit, Quantity: "1", Price: "1",
		},
	} {
		if err := caps.Check(order); err == nil {
			t.Errorf("%s must be rejected by the capability check", name)
		}
	}

	// The TWAP extra keys the client reads stay accepted.
	if err := caps.Check(ex.CreateOrderParams{
		Market: btc, Side: acct.OrderSideBuy, Type: acct.OrderTypeMarket, Price: "1",
		Quantity: "1", Extra: map[string]any{"twap": true, "minutes": 5, "randomize": true},
	}); err != nil {
		t.Errorf("twap extra keys must pass, got: %v", err)
	}
}

func assertSlice[T comparable](t *testing.T, what string, got, want []T) {
	t.Helper()

	if !slices.Equal(got, want) {
		t.Errorf("%s = %v, want %v", what, got, want)
	}
}
