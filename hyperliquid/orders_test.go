package hyperliquid

import (
	"testing"

	ex "github.com/plusev-terminal/go-plugin-common/v2/datasrc/exchange"
	acct "github.com/plusev-terminal/go-plugin-common/v2/datasrc/exchange/account"
	commonTesting "github.com/plusev-terminal/go-plugin-common/v2/requester/testing"
	tt "github.com/plusev-terminal/go-plugin-common/v2/trading"
)

// TestGetOrdersTriggerMapping pins the normalized shape of open orders:
// trigger prices are first-class on acct.Order (never Components), a
// Hyperliquid trigger order always reports the mark price as its source
// (the venue fires TP/SL on mark), and venue "0.0" placeholders never
// leak into price fields.
func TestGetOrdersTriggerMapping(t *testing.T) {
	mock := commonTesting.NewMockRequester()
	mock.SetMockResponse("/info", `[
		{
			"coin": "BTC", "side": "B", "limitPx": "42000", "oid": 1,
			"sz": "0.5", "origSz": "1", "orderType": "Limit", "reduceOnly": false,
			"timestamp": 1700000000000, "isTrigger": true,
			"triggerCondition": "U", "triggerPx": "43000", "isPositionTpsl": false
		},
		{
			"coin": "ETH", "side": "A", "limitPx": "2300", "oid": 2,
			"sz": "2", "origSz": "2", "orderType": "Limit", "reduceOnly": false,
			"timestamp": 1700000000000, "isTrigger": false,
			"triggerCondition": "N/A", "triggerPx": "0.0", "isPositionTpsl": false
		}
	]`)

	client := NewClient(mock, "https://api.hyperliquid.xyz", true)

	resp, err := client.GetOrders(true)
	if err != nil {
		t.Fatalf("GetOrders failed: %v", err)
	}

	orders := resp.Scopes[acct.ScopeFutures].Orders
	if len(orders) != 2 {
		t.Fatalf("expected 2 orders, got %d", len(orders))
	}

	byID := map[string]acct.Order{}
	for _, o := range orders {
		byID[o.OrderID] = o
	}

	triggered := byID["1"]
	if triggered.TriggerPrice != "43000" {
		t.Errorf("trigger order TriggerPrice = %q, want 43000", triggered.TriggerPrice)
	}
	if triggered.TriggerSource != acct.TriggerSourceMark {
		t.Errorf("trigger order TriggerSource = %q, want mark", triggered.TriggerSource)
	}
	if _, has := triggered.Components["trigger_px"]; has {
		t.Error("trigger price must not be written to Components[\"trigger_px\"]")
	}
	if triggered.Type != acct.OrderTypeStopLimit {
		t.Errorf("trigger order Type = %q, want stop_limit", triggered.Type)
	}
	if triggered.Price != "42000" {
		t.Errorf("trigger order Price = %q, want 42000", triggered.Price)
	}

	plain := byID["2"]
	if plain.TriggerPrice != "" {
		t.Errorf("non-trigger order TriggerPrice = %q, want empty (venue sent 0.0)", plain.TriggerPrice)
	}
	if plain.TriggerSource != "" {
		t.Errorf("non-trigger order TriggerSource = %q, want empty", plain.TriggerSource)
	}
}

// TestBuildTriggerOrderWire pins the creation-side trigger mapping: the
// first-class TriggerPrice (or the alias matching the order type) becomes
// wire triggerPx with the venue's tp/sl direction, and the created order
// carries TriggerPrice + TriggerSource mark.
func TestBuildTriggerOrderWire(t *testing.T) {
	client := NewClient(commonTesting.NewMockRequester(), "https://api.hyperliquid.xyz", true)

	market := tt.Market{Symbol: "BTC", AssetType: tt.AssetTypePerpetual}

	tests := []struct {
		name          string
		order         ex.CreateOrderParams
		wantTriggerPx string
		wantTpsl      string
		wantIsMarket  bool
		wantLimitPx   string
	}{
		{
			name: "stop market via triggerPrice",
			order: ex.CreateOrderParams{
				Market: market, Side: acct.OrderSideSell, Type: acct.OrderTypeStop,
				Quantity: "1", TriggerPrice: "41000",
			},
			wantTriggerPx: "41000", wantTpsl: "sl", wantIsMarket: true, wantLimitPx: "41000",
		},
		{
			name: "take profit limit via takeProfitPrice alias",
			order: ex.CreateOrderParams{
				Market: market, Side: acct.OrderSideBuy, Type: acct.OrderTypeTakeProfitLimit,
				Quantity: "1", TakeProfitPrice: "45000", Price: "45100",
			},
			wantTriggerPx: "45000", wantTpsl: "tp", wantIsMarket: false, wantLimitPx: "45100",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			wireType, limitPx, err := client.buildOrderType(tc.order)
			if err != nil {
				t.Fatalf("buildOrderType failed: %v", err)
			}

			if wireType.Limit != nil {
				t.Fatalf("trigger order must not carry a limit wire type: %+v", wireType.Limit)
			}

			if wireType.Trigger == nil {
				t.Fatalf("expected trigger wire type, got %+v", wireType)
			}

			got := wireType.Trigger
			if got.TriggerPx != tc.wantTriggerPx || got.Tpsl != tc.wantTpsl || got.IsMarket != tc.wantIsMarket {
				t.Errorf("wire trigger = %+v, want triggerPx=%s tpsl=%s isMarket=%v",
					got, tc.wantTriggerPx, tc.wantTpsl, tc.wantIsMarket)
			}

			if limitPx != tc.wantLimitPx {
				t.Errorf("wire limitPx = %q, want %q", limitPx, tc.wantLimitPx)
			}

			result := buildOrderResult(tc.order, "77", "")
			if result.TriggerPrice != tc.wantTriggerPx {
				t.Errorf("result order TriggerPrice = %q, want %q", result.TriggerPrice, tc.wantTriggerPx)
			}
			if result.TriggerSource != acct.TriggerSourceMark {
				t.Errorf("result order TriggerSource = %q, want mark", result.TriggerSource)
			}
		})
	}
}
