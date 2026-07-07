package hyperliquid

import (
	"fmt"
	"strings"
	"time"

	acct "github.com/plusev-terminal/go-plugin-common/datasrc/exchange/account"
)


// GetOrders returns the user's open orders.
func (c *Client) GetOrders(openOnly bool) (acct.OrdersResponse, error) {
	var orders []FrontendOpenOrder

	reqType := "frontendOpenOrders"
	if !openOnly {
		reqType = "frontendOpenOrders"
	}

	err := c.postInfo(InfoRequest{Type: reqType, User: c.walletAddress}, &orders)
	if err != nil {
		return acct.OrdersResponse{}, fmt.Errorf("failed to fetch open orders: %w", err)
	}

	fetchedAt := time.Now()

	result := make([]acct.Order, 0, len(orders))

	for _, o := range orders {
		side := normalizeHLSide(o.Side)
		orderType := normalizeHLType(o)
		status := normalizeHLStatus(o)

		remaining := o.Sz
		executed := subtractStr(o.OrigSz, o.Sz)

		order := acct.Order{
			OrderID:           fmt.Sprintf("%d", o.Oid),
			Symbol:            o.Coin,
			Side:              side,
			Type:              orderType,
			Status:            status,
			Price:             o.LimitPx,
			Quantity:          o.OrigSz,
			ExecutedQuantity:  executed,
			RemainingQuantity: remaining,
			ReduceOnly:        o.ReduceOnly,
			CreatedAt:         time.UnixMilli(o.Timestamp),
			UpdatedAt:         time.UnixMilli(o.Timestamp),
			Components:        map[string]string{},
			Extra:             map[string]any{},
		}

		if o.Cloid != nil && *o.Cloid != "" {
			order.ClientOrderID = *o.Cloid
		}

		if o.IsTrigger {
			order.Components["trigger_px"] = o.TriggerPx
			order.Components["trigger_condition"] = o.TriggerCondition
		}

		if o.IsPositionTpsl {
			order.Extra["is_position_tpsl"] = true
		}

		// Classify as spot or perp
		// Hyperliquid open orders include both spot and perps in frontendOpenOrders.
		// Spot coins use @N or NAME/USDC format; perp coins are plain names.
		if strings.HasPrefix(o.Coin, "@") || strings.Contains(o.Coin, "/") {
			order.Extra["market_type"] = "spot"
		} else {
			order.Extra["market_type"] = "perp"
		}

		result = append(result, order)
	}

	// Split into spot and futures scopes
	spotOrders := make([]acct.Order, 0)
	perpOrders := make([]acct.Order, 0)

	for _, o := range result {
		mt, _ := o.Extra["market_type"].(string)
		if mt == "spot" {
			spotOrders = append(spotOrders, o)
		} else {
			perpOrders = append(perpOrders, o)
		}
	}

	scopes := map[acct.ScopeType]acct.OrderScope{}

	if len(perpOrders) > 0 {
		scopes[acct.ScopeFutures] = acct.OrderScope{Orders: perpOrders, Extra: map[string]any{}}
	}

	if len(spotOrders) > 0 {
		scopes[acct.ScopeSpot] = acct.OrderScope{Orders: spotOrders, Extra: map[string]any{}}
	}

	resp := acct.OrdersResponse{
		FetchedAt: fetchedAt,
		Scopes:    scopes,
		Extra:     map[string]any{},
	}

	return resp, nil
}


func normalizeHLSide(side string) acct.OrderSide {
	s := strings.ToUpper(strings.TrimSpace(side))
	switch s {
	case "B", "BUY", "LONG":
		return acct.OrderSideBuy
	case "A", "SELL", "SHORT":
		return acct.OrderSideSell
	default:
		return acct.OrderSideUnknown
	}
}


func normalizeHLType(o FrontendOpenOrder) acct.OrderType {
	if o.IsTrigger {
		// Determine TP vs SL from trigger condition
		cond := strings.ToUpper(strings.TrimSpace(o.TriggerCondition))
		if strings.Contains(cond, "TP") || strings.Contains(cond, "TAKE PROFIT") {
			if strings.ToUpper(o.OrderType) == "LIMIT" {
				return acct.OrderTypeTakeProfitLimit
			}
			return acct.OrderTypeTakeProfitMarket
		}
		// Default to stop
		if strings.ToUpper(o.OrderType) == "LIMIT" {
			return acct.OrderTypeStopLimit
		}
		return acct.OrderTypeStopMarket
	}

	s := strings.ToUpper(strings.TrimSpace(o.OrderType))
	s = strings.ReplaceAll(s, " ", "_")
	switch s {
	case "LIMIT":
		return acct.OrderTypeLimit
	case "MARKET":
		return acct.OrderTypeMarket
	default:
		return acct.OrderType(strings.ToLower(s))
	}
}


func normalizeHLStatus(o FrontendOpenOrder) acct.OrderStatus {
	// frontendOpenOrders are by definition open
	return acct.OrderStatusNew
}
