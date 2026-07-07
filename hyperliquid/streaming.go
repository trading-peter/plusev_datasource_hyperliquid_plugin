package hyperliquid

import (
	"encoding/json"
	"fmt"
	"strings"

	commonstream "github.com/plusev-terminal/go-plugin-common/stream"
	"github.com/plusev-terminal/go-plugin-common/plugin"
)


// PrepareStream sets up a WebSocket stream for OHLCV candle data.
func (c *Client) PrepareStream(request plugin.StreamSetupRequest) (plugin.StreamSetupResponse, error) {
	streamContext, _ := request.Parameters["streamContext"].(map[string]any)
	if streamContext == nil {
		return plugin.StreamSetupResponse{Success: false, Error: "streamContext is required"}, nil
	}

	coin, _ := streamContext["coin"].(string)
	interval, _ := streamContext["interval"].(string)

	if strings.TrimSpace(coin) == "" || strings.TrimSpace(interval) == "" {
		return plugin.StreamSetupResponse{Success: false, Error: "streamContext.coin and streamContext.interval are required"}, nil
	}

	// Build subscription message
	subscribeMsg := map[string]any{
		"method": "subscribe",
		"subscription": map[string]any{
			"type":     "candle",
			"coin":     coin,
			"interval": interval,
		},
	}

	msgBytes, err := json.Marshal(subscribeMsg)
	if err != nil {
		return plugin.StreamSetupResponse{
			Success: false,
			Error:   fmt.Sprintf("failed to marshal subscribe message: %v", err),
		}, nil
	}

	response := plugin.StreamSetupResponse{
		Success:         true,
		WebSocketURL:    c.wsBaseURL,
		InitialMessages: []string{string(msgBytes)},
		StreamContext:   streamContext,
	}

	return response, nil
}


// WSCandleUpdate represents a candle update from the WebSocket.
type WSCandleUpdate struct {
	Channel string `json:"channel"`
	Data    struct {
		T int64  `json:"t"`
		O string `json:"o"`
		H string `json:"h"`
		L string `json:"l"`
		C string `json:"c"`
		V string `json:"v"`
		N int    `json:"n"`
	} `json:"data"`
}


// HandleStreamMessage processes incoming WebSocket messages and extracts candle data.
func (c *Client) HandleStreamMessage(request plugin.StreamMessageRequest) (plugin.StreamMessageResponse, error) {
	// Parse as candle update
	var update WSCandleUpdate

	if err := json.Unmarshal(request.Message, &update); err == nil {
		if update.Channel == "candle" {
			record := map[string]any{
				"openTime": update.Data.T / 1000,
				"open":     update.Data.O,
				"high":     update.Data.H,
				"low":      update.Data.L,
				"close":    update.Data.C,
				"volume":   update.Data.V,
				"isClosed": false,
			}

			return plugin.StreamMessageResponse{
				Success:  true,
				Action:   "data",
				DataType: "ohlcv",
				Data:     record,
			}, nil
		}
	}

	// Check if it's a subscription response
	var subResp map[string]any
	if err := json.Unmarshal(request.Message, &subResp); err == nil {
		if channel, _ := subResp["channel"].(string); channel == "subscriptionResponse" {
			return plugin.StreamMessageResponse{Success: true, Action: "ignore"}, nil
		}
	}

	return plugin.StreamMessageResponse{Success: true, Action: "ignore"}, nil
}


// HandleConnectionEvent handles WebSocket connection lifecycle events.
func (c *Client) HandleConnectionEvent(event plugin.StreamConnectionEvent) (plugin.StreamConnectionResponse, error) {
	switch event.EventType {
	case "disconnected":
		return plugin.StreamConnectionResponse{Success: true, Action: "reconnect"}, nil
	case "error":
		c.log.ErrorWithData("WebSocket error", map[string]any{
			"streamID": event.StreamID,
			"error":    event.Error,
		})
		return plugin.StreamConnectionResponse{Success: true, Action: "ignore"}, nil
	default:
		return plugin.StreamConnectionResponse{Success: true, Action: "ignore"}, nil
	}
}


// BuildStreamMarker creates the StreamMarker response for ohlcvStream command.
func BuildStreamMarker(setupResp plugin.StreamSetupResponse, streamID string) (commonstream.StreamMarker, error) {
	marker := commonstream.StreamMarker{
		Stream:          true,
		StreamID:        streamID,
		WebSocketURL:    setupResp.WebSocketURL,
		InitialMessages: setupResp.InitialMessages,
		StreamContext:   setupResp.StreamContext,
	}

	return marker, marker.Validate()
}
