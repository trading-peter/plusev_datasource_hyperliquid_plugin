package hyperliquid

import (
	"encoding/json"
	"fmt"
	"strconv"

	tt "github.com/plusev-terminal/go-plugin-common/trading"
)


// GetMarkets fetches all available perpetual and spot markets.
func (c *Client) GetMarkets() ([]tt.Market, error) {
	var markets []tt.Market

	perpMarkets, err := c.getPerpMarkets()
	if err != nil {
		return nil, err
	}

	markets = append(markets, perpMarkets...)

	spotMarkets, err := c.getSpotMarkets()
	if err != nil {
		// Spot metadata might not always be available; continue with perps only
		c.log.ErrorWithData("Failed to fetch spot markets", map[string]any{"error": err})
	} else {
		markets = append(markets, spotMarkets...)
	}

	return markets, nil
}


func (c *Client) getPerpMarkets() ([]tt.Market, error) {
	var raw json.RawMessage

	if err := c.postInfo(InfoRequest{Type: "metaAndAssetCtxs"}, &raw); err != nil {
		return nil, fmt.Errorf("failed to fetch perp markets: %w", err)
	}

	var arr []json.RawMessage
	if err := json.Unmarshal(raw, &arr); err != nil {
		return nil, fmt.Errorf("failed to parse metaAndAssetCtxs envelope: %w", err)
	}

	if len(arr) < 2 {
		return nil, fmt.Errorf("metaAndAssetCtxs returned incomplete array")
	}

	var meta MetaResponse
	if err := json.Unmarshal(arr[0], &meta); err != nil {
		return nil, fmt.Errorf("failed to parse perp meta: %w", err)
	}

	var ctxs []PerpAssetCtx
	if err := json.Unmarshal(arr[1], &ctxs); err != nil {
		return nil, fmt.Errorf("failed to parse perp asset contexts: %w", err)
	}

	// Build perp index cache
	c.perpIndexCache = make(map[string]int, len(meta.Universe))

	markets := make([]tt.Market, 0, len(meta.Universe))

	for i, asset := range meta.Universe {
		if asset.IsDelisted {
			continue
		}

		c.perpIndexCache[asset.Name] = i

		market := tt.Market{
			Label:       fmt.Sprintf("%s-PERP", asset.Name),
			Symbol:      asset.Name,
			Base:        asset.Name,
			Quote:       "USDC",
			AssetType:   "perpetual",
			MaxLeverage: strconv.Itoa(asset.MaxLeverage),
		}

		if asset.SzDecimals > 0 {
			tick := calculateTickFromDecimals(asset.SzDecimals)
			market.QuantityTick = tick
			market.QuantityPrecision = asset.SzDecimals
			market.MinQuantity = tick
		}

		market.MinNotional = "10"

		if i < len(ctxs) {
			ctx := ctxs[i]

			if ctx.MarkPx != "" {
				market.Status = "TRADING"
			}

			if ctx.Funding != "" {
				market.FundingInterval = 1
			}
		}

		markets = append(markets, market)
	}

	return markets, nil
}


func (c *Client) getSpotMarkets() ([]tt.Market, error) {
	var raw json.RawMessage

	if err := c.postInfo(InfoRequest{Type: "spotMetaAndAssetCtxs"}, &raw); err != nil {
		return nil, err
	}

	var arr []json.RawMessage
	if err := json.Unmarshal(raw, &arr); err != nil {
		return nil, err
	}

	if len(arr) < 2 {
		return nil, fmt.Errorf("spotMetaAndAssetCtxs returned incomplete array")
	}

	var meta SpotMetaResponse
	if err := json.Unmarshal(arr[0], &meta); err != nil {
		return nil, err
	}

	var ctxs []SpotAssetCtx
	if err := json.Unmarshal(arr[1], &ctxs); err != nil {
		return nil, err
	}

	// Build spot index cache
	c.spotIndexCache = make(map[string]int, len(meta.Universe))

	markets := make([]tt.Market, 0, len(meta.Universe))

	for _, pair := range meta.Universe {
		if len(pair.Tokens) < 2 {
			continue
		}

		baseIdx := pair.Tokens[0]
		quoteIdx := pair.Tokens[1]

		var baseName, quoteName string

		for _, token := range meta.Tokens {
			if token.Index == baseIdx {
				baseName = token.Name
			}
			if token.Index == quoteIdx {
				quoteName = token.Name
			}
		}

		if baseName == "" || quoteName == "" {
			continue
		}

		c.spotIndexCache[pair.Name] = 10000 + pair.Index

		// Build label from pair name or token names
		label := pair.Name
		if pair.IsCanonical && pair.Name == fmt.Sprintf("%s/%s", baseName, quoteName) {
			label = pair.Name
		}

		// For @N pairs, use token names for a friendlier label
		if label == "" || label[0] == '@' {
			label = fmt.Sprintf("%s/%s", baseName, quoteName)
		}

		market := tt.Market{
			Label:     label,
			Symbol:    pair.Name,
			Base:      baseName,
			Quote:     quoteName,
			AssetType: "spot",
		}

		// Set quantity precision from base token
		for _, token := range meta.Tokens {
			if token.Index == baseIdx && token.SzDecimals > 0 {
				tick := calculateTickFromDecimals(token.SzDecimals)
				market.QuantityTick = tick
				market.QuantityPrecision = token.SzDecimals
				market.MinQuantity = tick
			}
		}

		market.MinNotional = "10"

		if pair.Index < len(ctxs) {
			ctx := ctxs[pair.Index]

			if ctx.MarkPx != "" {
				market.Status = "TRADING"
			}
		}

		markets = append(markets, market)
	}

	return markets, nil
}


// GetTimeframes returns the supported candle intervals.
func (c *Client) GetTimeframes() []tt.Timeframe {
	return []tt.Timeframe{
		{Value: 1, Unit: tt.Minutes},
		{Value: 3, Unit: tt.Minutes},
		{Value: 5, Unit: tt.Minutes},
		{Value: 15, Unit: tt.Minutes},
		{Value: 30, Unit: tt.Minutes},
		{Value: 1, Unit: tt.Hours},
		{Value: 2, Unit: tt.Hours},
		{Value: 4, Unit: tt.Hours},
		{Value: 8, Unit: tt.Hours},
		{Value: 12, Unit: tt.Hours},
		{Value: 1, Unit: tt.Days},
		{Value: 3, Unit: tt.Days},
		{Value: 1, Unit: tt.Weeks},
		{Value: 1, Unit: tt.Months},
	}
}


// calculateTickFromDecimals returns the tick size for a given number of decimals.
func calculateTickFromDecimals(decimals int) string {
	if decimals <= 0 {
		return "1"
	}

	result := "0."
	for i := 1; i < decimals; i++ {
		result += "0"
	}

	return result + "1"
}


// currentTimestampMs returns the current time in milliseconds.
func currentTimestampMs() int64 {
	return timeNow().UnixMilli()
}
