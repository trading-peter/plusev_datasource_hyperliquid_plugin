package hyperliquid

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	ex "github.com/plusev-terminal/go-plugin-common/datasrc/exchange"
	acct "github.com/plusev-terminal/go-plugin-common/datasrc/exchange/account"
	tt "github.com/plusev-terminal/go-plugin-common/trading"
)


// CreateOrderResult holds the outcome of creating a single order.
type CreateOrderResult struct {
	Index         int
	Success       bool
	Error         string
	OrderID       string
	ClientOrderID string
	Order         *acct.Order
	Extra         map[string]any
}


// indexedOrder pairs an order with its original index in the batch.
type indexedOrder struct {
	index int
	order ex.CreateOrderParams
}


// CreateOrders places one or more orders on Hyperliquid.
func (c *Client) CreateOrders(params ex.CreateOrdersParams) (acct.CreateOrdersResponse, error) {
	results := make([]CreateOrderResult, len(params.Orders))
	for i := range results {
		results[i] = CreateOrderResult{Index: i}
	}

	// Separate TWAP orders from regular orders
	regularOrders := make([]indexedOrder, 0)
	twapOrders := make([]indexedOrder, 0)

	for i, order := range params.Orders {
		isTwap := false
		if order.Extra != nil {
			if v, ok := order.Extra["twap"]; ok {
				if b, ok := v.(bool); ok && b {
					isTwap = true
				}
			}
		}

		if isTwap {
			twapOrders = append(twapOrders, indexedOrder{index: i, order: order})
		} else {
			regularOrders = append(regularOrders, indexedOrder{index: i, order: order})
		}
	}

	// Handle regular orders as a batch
	if len(regularOrders) > 0 {
		err := c.handleRegularOrders(regularOrders, results)
		if err != nil {
			return acct.CreateOrdersResponse{}, err
		}
	}

	// Handle TWAP orders individually
	for _, item := range twapOrders {
		err := c.handleTwapOrder(item.order, &results[item.index])
		if err != nil {
			results[item.index].Success = false
			results[item.index].Error = err.Error()
		}
	}

	// Convert to response
	respResults := make([]acct.CreateOrderResult, len(results))
	for i, r := range results {
		result := acct.CreateOrderResult{
			Index:         r.Index,
			ClientOrderID: r.ClientOrderID,
			Success:       r.Success,
			Error:         r.Error,
			Order:         r.Order,
			Extra:         r.Extra,
		}
		respResults[i] = result
	}

	return acct.CreateOrdersResponse{
		SubmittedAt: time.Now(),
		Results:     respResults,
	}, nil
}


func (c *Client) handleRegularOrders(orders []indexedOrder, results []CreateOrderResult) error {
	wires := make([]OrderWire, 0, len(orders))
	orderRefs := make([]indexedOrder, 0, len(orders))

	for _, item := range orders {
		wire, err := c.buildOrderWire(item.order)
		if err != nil {
			results[item.index].Success = false
			results[item.index].Error = err.Error()
			continue
		}

		wires = append(wires, wire)
		orderRefs = append(orderRefs, item)
	}

	if len(wires) == 0 {
		return nil
	}

	// Determine grouping
	grouping := "na"

	// Check if any orders have TP/SL triggers
	hasTrigger := false
	for _, w := range wires {
		if w.OrderType.Trigger != nil {
			hasTrigger = true
			break
		}
	}

	if hasTrigger {
		grouping = "normalTpsl"
	}

	action := OrderAction{
		Type:     "order",
		Orders:   wires,
		Grouping: grouping,
	}

	var respRaw json.RawMessage
	if err := c.postExchange(action, &respRaw); err != nil {
		// If the whole batch failed, mark all non-failed orders as errored
		for _, ref := range orderRefs {
			if !results[ref.index].Success && results[ref.index].Error == "" {
				results[ref.index].Success = false
				results[ref.index].Error = err.Error()
			}
		}
		return nil
	}

	// Parse the response
	var exchResp ExchangeResponse
	if err := json.Unmarshal(respRaw, &exchResp); err != nil {
		return fmt.Errorf("failed to parse exchange response: %w", err)
	}

	if exchResp.Status != "ok" {
		return fmt.Errorf("exchange returned status %s", exchResp.Status)
	}

	var orderResp OrderActionResponse
	if err := json.Unmarshal(exchResp.Response, &orderResp); err != nil {
		return fmt.Errorf("failed to parse order response: %w", err)
	}

	// Map statuses back to individual orders
	for i, status := range orderResp.Data.Statuses {
		if i >= len(orderRefs) {
			break
		}

		ref := orderRefs[i]
		result := &results[ref.index]

		if status.Error != nil {
			result.Success = false
			result.Error = *status.Error
			continue
		}

		result.Success = true

		orderID := ""
		avgPx := ""

		if status.Resting != nil {
			orderID = fmt.Sprintf("%d", status.Resting.Oid)
		} else if status.Filled != nil {
			orderID = fmt.Sprintf("%d", status.Filled.Oid)
			avgPx = status.Filled.AvgPx
		}

		result.OrderID = orderID
		result.ClientOrderID = ref.order.ClientOrderID

		result.Order = buildOrderResult(ref.order, orderID, avgPx)

		result.Extra = map[string]any{
			"order_type": string(ref.order.Type),
		}

		if status.Resting != nil {
			result.Extra["status"] = "resting"
		} else if status.Filled != nil {
			result.Extra["status"] = "filled"
			result.Extra["filled_size"] = status.Filled.TotalSz
			result.Extra["avg_px"] = avgPx
		}
	}

	return nil
}


func (c *Client) handleTwapOrder(order ex.CreateOrderParams, result *CreateOrderResult) error {
	isSpot := isSpotMarket(order.Market)
	asset, err := c.coinToAssetIndex(order.Market.Symbol, isSpot)
	if err != nil {
		return err
	}

	sizeWire, err := floatToWireStr(parseFloatStr(parseQty(order)))
	if err != nil {
		return fmt.Errorf("invalid size for TWAP: %w", err)
	}

	minutes := 5
	if order.Extra != nil {
		if v, ok := extraInt(order.Extra, "minutes"); ok {
			minutes = v
		}
	}

	randomize := false
	if order.Extra != nil {
		if v, ok := extraBool(order.Extra, "randomize"); ok {
			randomize = v
		}
	}

	action := TwapOrderAction{
		Type: "twapOrder",
		Twap: TwapOrderWire{
			Asset:      asset,
			IsBuy:      order.Side == acct.OrderSideBuy,
			Size:       sizeWire,
			ReduceOnly: order.ReduceOnly,
			Minutes:    minutes,
			Randomize:  randomize,
		},
	}

	var respRaw json.RawMessage
	if err := c.postExchange(action, &respRaw); err != nil {
		return err
	}

	var exchResp ExchangeResponse
	if err := json.Unmarshal(respRaw, &exchResp); err != nil {
		return fmt.Errorf("failed to parse TWAP response: %w", err)
	}

	result.Success = true
	result.ClientOrderID = order.ClientOrderID
	result.Order = buildOrderResult(order, "", "")
	result.Extra = map[string]any{
		"twap":     true,
		"minutes":  minutes,
		"response": string(exchResp.Response),
	}

	return nil
}


// buildOrderWire converts a normalized order to the Hyperliquid wire format.
func (c *Client) buildOrderWire(order ex.CreateOrderParams) (OrderWire, error) {
	isSpot := isSpotMarket(order.Market)
	asset, err := c.coinToAssetIndex(order.Market.Symbol, isSpot)
	if err != nil {
		return OrderWire{}, err
	}

	isBuy := order.Side == acct.OrderSideBuy

	sizeWire, err := floatToWireStr(parseFloatStr(parseQty(order)))
	if err != nil {
		return OrderWire{}, fmt.Errorf("invalid size: %w", err)
	}

	// Determine order type and build wire type
	wireType, priceWire, err := c.buildOrderType(order)
	if err != nil {
		return OrderWire{}, err
	}

	wire := OrderWire{
		Asset:      asset,
		IsBuy:      isBuy,
		LimitPx:    priceWire,
		Size:       sizeWire,
		ReduceOnly: order.ReduceOnly,
		OrderType:  wireType,
	}

	if order.ClientOrderID != "" {
		cloid := normalizeCloid(order.ClientOrderID)
		wire.Cloid = &cloid
	}

	return wire, nil
}


// buildOrderType determines the wire order type and price based on the normalized order.
func (c *Client) buildOrderType(order ex.CreateOrderParams) (OrderWireType, string, error) {
	switch order.Type {
	case acct.OrderTypeLimit:
		tif := "Gtc"
		if order.PostOnly || order.TimeInForce == acct.TimeInForcePostOnly || order.TimeInForce == acct.TimeInForceGTX {
			tif = "Alo"
		} else if order.TimeInForce == acct.TimeInForceIOC {
			tif = "Ioc"
		}

		if order.Price == "" {
			return OrderWireType{}, "", fmt.Errorf("price is required for limit order")
		}

		priceWire, err := floatToWireStr(parseFloatStr(order.Price))
		if err != nil {
			return OrderWireType{}, "", fmt.Errorf("invalid limit price: %w", err)
		}

		return OrderWireType{
			Limit: &OrderWireLimit{Tif: tif},
		}, priceWire, nil

	case acct.OrderTypeMarket:
		if order.Price == "" {
			return OrderWireType{}, "", fmt.Errorf("price (market estimate) is required for market order")
		}

		priceWire, err := floatToWireStr(parseFloatStr(order.Price))
		if err != nil {
			return OrderWireType{}, "", fmt.Errorf("invalid market price: %w", err)
		}

		return OrderWireType{
			Limit: &OrderWireLimit{Tif: "Ioc"},
		}, priceWire, nil

	case acct.OrderTypeStop, acct.OrderTypeStopMarket:
		return c.buildTriggerOrder(order, "sl")

	case acct.OrderTypeTakeProfit, acct.OrderTypeTakeProfitMarket:
		return c.buildTriggerOrder(order, "tp")

	case acct.OrderTypeStopLimit:
		return c.buildTriggerOrder(order, "sl")

	case acct.OrderTypeTakeProfitLimit:
		return c.buildTriggerOrder(order, "tp")

	default:
		return OrderWireType{}, "", fmt.Errorf("unsupported order type: %s", order.Type)
	}
}


// buildTriggerOrder builds a trigger (TP/SL) order wire type.
func (c *Client) buildTriggerOrder(order ex.CreateOrderParams, tpsl string) (OrderWireType, string, error) {
	triggerPxStr := order.TriggerPrice
	if triggerPxStr == "" {
		if tpsl == "tp" {
			triggerPxStr = order.TakeProfitPrice
		} else {
			triggerPxStr = order.StopPrice
		}
	}

	if triggerPxStr == "" {
		return OrderWireType{}, "", fmt.Errorf("triggerPrice is required for trigger order")
	}

	triggerPx, err := floatToWireStr(parseFloatStr(triggerPxStr))
	if err != nil {
		return OrderWireType{}, "", fmt.Errorf("invalid trigger price: %w", err)
	}

	isMarket := order.Type == acct.OrderTypeStop ||
		order.Type == acct.OrderTypeStopMarket ||
		order.Type == acct.OrderTypeTakeProfit ||
		order.Type == acct.OrderTypeTakeProfitMarket

	// Limit price is required for limit trigger orders, used as limitPx for the wire
	limitPx := triggerPx

	if !isMarket {
		if order.Price == "" {
			return OrderWireType{}, "", fmt.Errorf("limit price is required for limit trigger order")
		}

		limitPx, err = floatToWireStr(parseFloatStr(order.Price))
		if err != nil {
			return OrderWireType{}, "", fmt.Errorf("invalid limit price for trigger: %w", err)
		}
	}

	return OrderWireType{
		Trigger: &OrderWireTrigger{
			IsMarket:  isMarket,
			TriggerPx: triggerPx,
			Tpsl:      tpsl,
		},
	}, limitPx, nil
}


func buildOrderResult(order ex.CreateOrderParams, orderID string, avgPx string) *acct.Order {
	return &acct.Order{
		OrderID:       orderID,
		ClientOrderID: order.ClientOrderID,
		Symbol:        order.Market.Symbol,
		Side:          order.Side,
		Type:          order.Type,
		Price:         order.Price,
		Quantity:      parseQty(order),
		ReduceOnly:    order.ReduceOnly,
		PostOnly:      order.PostOnly,
	}
}


// parseQty extracts the quantity string from an order.
func parseQty(order ex.CreateOrderParams) string {
	if order.Quantity != "" {
		return order.Quantity
	}

	return order.QuoteQuantity
}


// isSpotMarket returns true if the market asset type is spot.
func isSpotMarket(market tt.Market) bool {
	return market.AssetType == "spot"
}


// floatToWireStr converts a float64 to a wire-compatible string (strips trailing zeros).
func floatToWireStr(x float64) (string, error) {
	if x == 0 {
		return "0", nil
	}

	// Format to 8 decimal places
	formatted := fmt.Sprintf("%.8f", x)

	// Handle -0
	if formatted == "-0.00000000" {
		formatted = "0.00000000"
	}

	// Check for significant rounding
	parsed, err := strconv.ParseFloat(formatted, 64)
	if err != nil {
		return "", err
	}

	if math.Abs(parsed-x) >= 1e-12 {
		return "", fmt.Errorf("float_to_wire causes rounding: %f", x)
	}

	// Strip trailing zeros
	result := strings.TrimRight(formatted, "0")
	result = strings.TrimRight(result, ".")

	return result, nil
}


func parseFloatStr(s string) float64 {
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0
	}

	return f
}


func formatFloat(f float64) string {
	return fmt.Sprintf("%.15g", f)
}


func absFloat(x float64) float64 {
	if x < 0 {
		return -x
	}

	return x
}


func extraInt(extra map[string]any, keys ...string) (int, bool) {
	for _, key := range keys {
		if v, ok := extra[key]; ok {
			switch t := v.(type) {
			case int:
				return t, true
			case int64:
				return int(t), true
			case float64:
				return int(t), true
			}
		}
	}

	return 0, false
}


func extraBool(extra map[string]any, keys ...string) (bool, bool) {
	for _, key := range keys {
		if v, ok := extra[key]; ok {
			if b, ok := v.(bool); ok {
				return b, true
			}
		}
	}

	return false, false
}
