package hyperliquid

import (
	"fmt"
	"time"

	acct "github.com/plusev-terminal/go-plugin-common/datasrc/exchange/account"
)


// GetPositions returns all open perpetual positions.
func (c *Client) GetPositions() (acct.PositionsResponse, error) {
	clearinghouse, err := c.getClearinghouseState()
	if err != nil {
		return acct.PositionsResponse{}, err
	}

	fetchedAt := time.Now()
	if clearinghouse.Time > 0 {
		fetchedAt = time.UnixMilli(clearinghouse.Time)
	}

	positions := make([]acct.Position, 0, len(clearinghouse.AssetPositions))

	for _, assetPos := range clearinghouse.AssetPositions {
		pos := assetPos.Position

		side := ""
		szi := parseFloatStr(pos.Szi)

		if szi > 0 {
			side = "long"
		} else if szi < 0 {
			side = "short"
		} else {
			continue
		}

		isIsolated := pos.Leverage.Type == "isolated"

		position := acct.Position{
			Symbol:          pos.Coin,
			Side:            side,
			Quantity:        fmt.Sprintf("%.15g", absFloat(szi)),
			EntryPrice:      pos.EntryPx,
			LiquidationPrice: pos.LiquidationPx,
			UnrealizedPnL:   pos.UnrealizedPnl,
			Leverage:        pos.Leverage.Value,
			IsIsolated:      isIsolated,
			InitialMargin:   pos.MarginUsed,
			Components: map[string]string{
				"position_value":  pos.PositionValue,
				"return_on_equity": pos.ReturnOnEquity,
				"max_leverage":    fmt.Sprintf("%d", pos.MaxLeverage),
			},
		}

		if pos.CumFunding != nil {
			position.Components["cum_funding_all_time"] = pos.CumFunding.AllTime
			position.Components["cum_funding_since_change"] = pos.CumFunding.SinceChange
		}

		positions = append(positions, position)
	}

	scope := acct.PositionScope{
		Positions: positions,
		Extra:     map[string]any{},
	}

	resp := acct.PositionsResponse{
		FetchedAt: fetchedAt,
		Scopes: map[acct.ScopeType]acct.PositionScope{
			acct.ScopeFutures: scope,
		},
		Extra: map[string]any{},
	}

	return resp, nil
}
