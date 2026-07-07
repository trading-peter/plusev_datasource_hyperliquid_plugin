package hyperliquid

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	ex "github.com/plusev-terminal/go-plugin-common/datasrc/exchange"
	acct "github.com/plusev-terminal/go-plugin-common/datasrc/exchange/account"
)


// CancelOrders cancels one or more orders, or all orders if no IDs are provided.
func (c *Client) CancelOrders(params ex.CancelOrdersParams) (acct.CancelOrdersResponse, error) {
	results := make([]acct.CancelOrderResult, 0)

	if len(params.IDs) == 0 {
		err := c.cancelAllOrders(&results, params.Market.Symbol)
		if err != nil {
			return acct.CancelOrdersResponse{}, err
		}
	} else {
		for i, id := range params.IDs {
			result := acct.CancelOrderResult{Index: i}

			err := c.cancelSingleOrder(strings.TrimSpace(id), params.Market.Symbol, &result)
			if err != nil {
				result.Success = false
				result.Error = err.Error()
			}

			results = append(results, result)
		}
	}

	return acct.CancelOrdersResponse{
		SubmittedAt: time.Now(),
		Results:     results,
	}, nil
}


// cancelAllOrders fetches all open orders and cancels them.
func (c *Client) cancelAllOrders(results *[]acct.CancelOrderResult, symbol string) error {
	var openOrders []FrontendOpenOrder

	err := c.postInfo(InfoRequest{Type: "frontendOpenOrders", User: c.walletAddress}, &openOrders)
	if err != nil {
		return fmt.Errorf("failed to fetch open orders for cancel-all: %w", err)
	}

	toCancel := make([]FrontendOpenOrder, 0)
	for _, o := range openOrders {
		if symbol != "" && o.Coin != symbol {
			continue
		}

		toCancel = append(toCancel, o)
	}

	if len(toCancel) == 0 {
		*results = append(*results, acct.CancelOrderResult{
			Index:   0,
			Success: true,
			Extra:   map[string]any{"note": "no open orders to cancel"},
		})
		return nil
	}

	for i, o := range toCancel {
		result := acct.CancelOrderResult{Index: i}
		result.OrderID = fmt.Sprintf("%d", o.Oid)

		isSpot := strings.HasPrefix(o.Coin, "@") || strings.Contains(o.Coin, "/")
		asset, err := c.coinToAssetIndex(o.Coin, isSpot)
		if err != nil {
			result.Success = false
			result.Error = err.Error()
			*results = append(*results, result)
			continue
		}

		action := CancelAction{
			Type: "cancel",
			Cancels: []CancelWire{
				{Asset: asset, OrderID: o.Oid},
			},
		}

		err = c.executeCancel(action, &result)
		if err != nil {
			result.Success = false
			result.Error = err.Error()
		}

		*results = append(*results, result)
	}

	return nil
}


func (c *Client) cancelSingleOrder(id, symbol string, result *acct.CancelOrderResult) error {
	isNumeric := true
	for _, r := range id {
		if r < '0' || r > '9' {
			isNumeric = false
			break
		}
	}

	isSpot := strings.HasPrefix(symbol, "@") || strings.Contains(symbol, "/")

	if isNumeric {
		oid, err := parseInt64(id)
		if err != nil {
			return fmt.Errorf("invalid order ID: %w", err)
		}

		result.OrderID = id

		asset, err := c.coinToAssetIndex(symbol, isSpot)
		if err != nil {
			return err
		}

		action := CancelAction{
			Type: "cancel",
			Cancels: []CancelWire{
				{Asset: asset, OrderID: oid},
			},
		}

		return c.executeCancel(action, result)
	}

	// Cancel by cloid
	result.ClientOrderID = id

	cloid := normalizeCloid(id)

	asset, err := c.coinToAssetIndex(symbol, isSpot)
	if err != nil {
		return err
	}

	action := CancelByCloidAction{
		Type: "cancelByCloid",
		Cancels: []CancelByCloidWire{
			{Asset: asset, Cloid: cloid},
		},
	}

	return c.executeCancelByCloid(action, result)
}


func (c *Client) executeCancel(action CancelAction, result *acct.CancelOrderResult) error {
	var respRaw json.RawMessage

	if err := c.postExchange(action, &respRaw); err != nil {
		return err
	}

	var exchResp ExchangeResponse
	if err := json.Unmarshal(respRaw, &exchResp); err != nil {
		return fmt.Errorf("failed to parse cancel response: %w", err)
	}

	if exchResp.Status != "ok" {
		return fmt.Errorf("exchange returned status %s", exchResp.Status)
	}

	var cancelResp struct {
		Data struct {
			Statuses []json.RawMessage `json:"statuses"`
		} `json:"data"`
	}

	if err := json.Unmarshal(exchResp.Response, &cancelResp); err != nil {
		result.Success = true
		return nil
	}

	if len(cancelResp.Data.Statuses) > 0 {
		statusStr := string(cancelResp.Data.Statuses[0])

		if strings.Contains(statusStr, "error") {
			var errResp struct {
				Error string `json:"error"`
			}

			if json.Unmarshal(cancelResp.Data.Statuses[0], &errResp) == nil && errResp.Error != "" {
				return fmt.Errorf("%s", errResp.Error)
			}
		}
	}

	result.Success = true
	return nil
}


func (c *Client) executeCancelByCloid(action CancelByCloidAction, result *acct.CancelOrderResult) error {
	var respRaw json.RawMessage

	if err := c.postExchange(action, &respRaw); err != nil {
		return err
	}

	var exchResp ExchangeResponse
	if err := json.Unmarshal(respRaw, &exchResp); err != nil {
		return fmt.Errorf("failed to parse cancelByCloid response: %w", err)
	}

	if exchResp.Status != "ok" {
		return fmt.Errorf("exchange returned status %s", exchResp.Status)
	}

	result.Success = true
	return nil
}


func parseInt64(s string) (int64, error) {
	var n int64
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}
