package hyperliquid

import (
	"fmt"
	"time"

	tt "github.com/plusev-terminal/go-plugin-common/v2/trading"
)

// OHLCVParams contains parameters for fetching candle data.
type OHLCVParams struct {
	Coin      string
	Interval  string
	StartTime *time.Time
	EndTime   *time.Time
	Limit     int
}

// GetOHLCV fetches historical candle data from the candleSnapshot endpoint.
func (c *Client) GetOHLCV(params OHLCVParams) ([]tt.OHLCVRecord, error) {
	req := map[string]any{
		"coin":     params.Coin,
		"interval": params.Interval,
	}

	if params.StartTime != nil {
		req["startTime"] = params.StartTime.UnixMilli()
	}

	if params.EndTime != nil {
		req["endTime"] = params.EndTime.UnixMilli()
	}

	body := InfoRequest{
		Type: "candleSnapshot",
		Req:  req,
	}

	var candles []CandleSnapshot
	if err := c.postInfo(body, &candles); err != nil {
		return nil, fmt.Errorf("failed to fetch candle snapshot: %w", err)
	}

	// Hyperliquid returns newest-first; reverse for chronological order
	records := make([]tt.OHLCVRecord, 0, len(candles))

	for i := len(candles) - 1; i >= 0; i-- {
		candle := candles[i]

		record := tt.OHLCVRecord{
			OpenTime: candle.T / 1000,
			Open:     candle.O,
			High:     candle.H,
			Low:      candle.L,
			Close:    candle.C,
			Volume:   candle.V,
			IsClosed: true,
		}

		// Last candle may not be closed
		if i == 0 {
			record.IsClosed = false
		}

		records = append(records, record)
	}

	// Apply limit if specified
	if params.Limit > 0 && len(records) > params.Limit {
		start := len(records) - params.Limit
		records = records[start:]
	}

	return records, nil
}

// ConvertTimeframeToInterval converts our timeframe string to Hyperliquid interval format.
// Hyperliquid uses: "1m", "3m", "5m", "15m", "30m", "1h", "2h", "4h", "8h", "12h", "1d", "3d", "1w", "1M"
func ConvertTimeframeToInterval(timeframe string) string {
	// Our timeframe format is already compatible (e.g., "1m", "1h", "1d", "1W", "1M")
	// Hyperliquid expects lowercase for m/h/d/w, uppercase M for month
	tf, err := tt.TimeframeFromString(timeframe)
	if err != nil {
		return "1h"
	}

	switch tf.Unit {
	case tt.Minutes:
		return fmt.Sprintf("%dm", int(tf.Value))
	case tt.Hours:
		return fmt.Sprintf("%dh", int(tf.Value))
	case tt.Days:
		return fmt.Sprintf("%dd", int(tf.Value))
	case tt.Weeks:
		return fmt.Sprintf("%dw", int(tf.Value))
	case tt.Months:
		return fmt.Sprintf("%dM", int(tf.Value))
	default:
		return "1h"
	}
}
