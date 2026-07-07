package main

import (
	"strings"
	"time"

	"github.com/plusev-terminal/go-plugin-common/plugin"
	"github.com/plusev-terminal/go-plugin-common/requester"

	"github.com/trading-peter/plusev_datasource_hyperliquid_plugin/hyperliquid"

	ex "github.com/plusev-terminal/go-plugin-common/datasrc/exchange"
	m "github.com/plusev-terminal/go-plugin-common/meta"
)


func init() {
	p := &HyperliquidPlugin{}

	plugin.RegisterPlugin(p)
	plugin.RegisterStreamHandler(&streamHandlerWrapper{p: p})
}


func main() {}


// ============================================================================
// Plugin Implementation
// ============================================================================


type HyperliquidPlugin struct {
	config *plugin.ConfigStore
	client *hyperliquid.Client
}


func (p *HyperliquidPlugin) GetMeta() m.Meta {
	return m.Meta{
		PluginID:    "hyperliquid-datasource",
		Name:        "Hyperliquid",
		AppID:       "datasrc",
		Category:    "dex",
		Description: "Hyperliquid L1 exchange — perps and spot with agent wallet trading",
		Author:      "trading_peter",
		Version:     "0.1.0",
		Repository:  "https://github.com/trading-peter/plusev_datasource_hyperliquid_plugin",
		Tags:        []string{"hyperliquid", "dex", "perps", "spot", "l1"},
		Contacts: []m.AuthorContact{
			{Kind: "x.com", Value: "https://x.com/trading_peter"},
		},
		Resources: m.ResourceAccess{
			AllowedNetworkTargets: []m.NetworkTargetRule{
				{Pattern: "https://api.hyperliquid.xyz/*"},
				{Pattern: "https://api.hyperliquid-testnet.xyz/*"},
				{Pattern: "wss://api.hyperliquid.xyz/*"},
				{Pattern: "wss://api.hyperliquid-testnet.xyz/*"},
			},
		},
		Features: []string{},
	}
}


func (p *HyperliquidPlugin) GetRateLimits() []plugin.RateLimit {
	return []plugin.RateLimit{
		{Command: ex.CMD_GET_MARKETS, Scope: []plugin.RateLimitScope{plugin.RateLimitScopeIP}, RPS: 2, Burst: 2},
		{Command: ex.CMD_GET_TIMEFRAMES, Scope: []plugin.RateLimitScope{plugin.RateLimitScopeIP}, RPS: 10, Burst: 10},
		{Command: ex.CMD_GET_OHLCV, Scope: []plugin.RateLimitScope{plugin.RateLimitScopeIP}, RPS: 5, Burst: 5},
		{Command: ex.CMD_OHLCV_STREAM, Scope: []plugin.RateLimitScope{plugin.RateLimitScopeIP}, RPS: 1, Burst: 5},
		{Command: ex.CMD_GET_ACCOUNT, Scope: []plugin.RateLimitScope{plugin.RateLimitScopeAPIKey}, RPS: 2, Burst: 2},
		{Command: ex.CMD_GET_BALANCES, Scope: []plugin.RateLimitScope{plugin.RateLimitScopeAPIKey}, RPS: 2, Burst: 2},
		{Command: ex.CMD_GET_POSITIONS, Scope: []plugin.RateLimitScope{plugin.RateLimitScopeAPIKey}, RPS: 2, Burst: 2},
		{Command: ex.CMD_GET_ORDERS, Scope: []plugin.RateLimitScope{plugin.RateLimitScopeAPIKey}, RPS: 2, Burst: 2},
		{Command: ex.CMD_CREATE_ORDERS, Scope: []plugin.RateLimitScope{plugin.RateLimitScopeAPIKey}, RPS: 5, Burst: 5},
		{Command: ex.CMD_CANCEL_ORDERS, Scope: []plugin.RateLimitScope{plugin.RateLimitScopeAPIKey}, RPS: 5, Burst: 5},
	}
}


func (p *HyperliquidPlugin) GetConfigFields() []plugin.ConfigField {
	if p.client == nil {
		p.client = hyperliquid.NewClient(requester.NewRequester(), "https://api.hyperliquid.xyz", true)
	}

	return p.client.GetConfigFields()
}


func (p *HyperliquidPlugin) OnInit(config *plugin.ConfigStore) error {
	p.config = config

	isTestnet := config.GetBool("testnet")

	baseURL := "https://api.hyperliquid.xyz"
	if isTestnet {
		baseURL = "https://api.hyperliquid-testnet.xyz"
	}

	p.client = hyperliquid.NewClient(requester.NewRequester(), baseURL, !isTestnet)

	credentials := make(map[string]string)

	if addr := config.GetString("walletAddress"); addr != "" {
		credentials["walletAddress"] = addr
	}

	if pk := config.GetString("privateKey"); pk != "" {
		credentials["privateKey"] = pk
	}

	p.client.SetCredentials(credentials)

	return nil
}


func (p *HyperliquidPlugin) OnShutdown() error {
	return nil
}


func (p *HyperliquidPlugin) RegisterCommands(router *plugin.CommandRouter) {
	router.Register(ex.CMD_GET_MARKETS, p.handleGetMarkets)
	router.Register(ex.CMD_GET_TIMEFRAMES, p.handleGetTimeframes)
	router.Register(ex.CMD_GET_OHLCV, p.handleGetOHLCV)
	router.Register(ex.CMD_OHLCV_STREAM, p.handleOHLCVStream)
	router.Register(ex.CMD_GET_ACCOUNT, p.handleGetAccount)
	router.Register(ex.CMD_GET_BALANCES, p.handleGetBalances)
	router.Register(ex.CMD_GET_POSITIONS, p.handleGetPositions)
	router.Register(ex.CMD_GET_ORDERS, p.handleGetOrders)
	router.Register(ex.CMD_CREATE_ORDERS, p.handleCreateOrders)
	router.Register(ex.CMD_CANCEL_ORDERS, p.handleCancelOrders)
}


// ============================================================================
// Stream Handler Wrapper
// ============================================================================


type streamHandlerWrapper struct {
	p *HyperliquidPlugin
}


func (w *streamHandlerWrapper) HandleStreamMessage(req plugin.StreamMessageRequest) (plugin.StreamMessageResponse, error) {
	if w.p.client == nil {
		return plugin.StreamMessageResponse{Success: false, Action: "ignore"}, nil
	}

	return w.p.client.HandleStreamMessage(req)
}


func (w *streamHandlerWrapper) HandleConnectionEvent(event plugin.StreamConnectionEvent) (plugin.StreamConnectionResponse, error) {
	if w.p.client == nil {
		return plugin.StreamConnectionResponse{Success: false, Action: "ignore"}, nil
	}

	return w.p.client.HandleConnectionEvent(event)
}


// ============================================================================
// Command Handlers
// ============================================================================


func (p *HyperliquidPlugin) handleGetMarkets(_ map[string]any) plugin.Response {
	markets, err := p.client.GetMarkets()
	if err != nil {
		return plugin.ErrorResponse(err)
	}

	return plugin.SuccessResponse(markets, time.Hour*12)
}


func (p *HyperliquidPlugin) handleGetTimeframes(_ map[string]any) plugin.Response {
	timeframes := p.client.GetTimeframes()

	return plugin.SuccessResponse(timeframes, time.Hour*12)
}


func (p *HyperliquidPlugin) handleGetOHLCV(params map[string]any) plugin.Response {
	if p.client == nil {
		return plugin.ErrorResponseMsg("client not initialized")
	}

	ohlcvParams := ex.GetOHLCVParamsFromMap(params)

	interval := hyperliquid.ConvertTimeframeToInterval(ohlcvParams.Timeframe)

	candleParams := hyperliquid.OHLCVParams{
		Coin:      ohlcvParams.Market.Symbol,
		Interval:  interval,
		StartTime: ohlcvParams.StartTime,
		EndTime:   ohlcvParams.EndTime,
		Limit:     ohlcvParams.Limit,
	}

	records, err := p.client.GetOHLCV(candleParams)
	if err != nil {
		return plugin.ErrorResponse(err)
	}

	if ohlcvParams.CacheForSeconds > 0 {
		return plugin.SuccessResponse(records, time.Duration(ohlcvParams.CacheForSeconds)*time.Second)
	}

	return plugin.SuccessResponse(records)
}


func (p *HyperliquidPlugin) handleOHLCVStream(params map[string]any) plugin.Response {
	if p.client == nil {
		return plugin.ErrorResponseMsg("client not initialized")
	}

	streamParams := ex.OHLCVStreamParamsFromMap(params)
	if strings.TrimSpace(streamParams.Market.Symbol) == "" {
		return plugin.ErrorResponseMsg("market.symbol is required")
	}

	interval := hyperliquid.ConvertTimeframeToInterval(streamParams.Timeframe)

	streamReq := plugin.StreamSetupRequest{
		StreamID:   "hyperliquid_ohlcv_" + streamParams.Market.Symbol + "_" + streamParams.Timeframe,
		StreamType: "ohlcv",
		Parameters: map[string]any{
			"market":    streamParams.Market,
			"timeframe": streamParams.Timeframe,
			"private":   false,
			"streamContext": map[string]any{
				"coin":     streamParams.Market.Symbol,
				"interval": interval,
			},
		},
	}

	setupResp, err := p.client.PrepareStream(streamReq)
	if err != nil {
		return plugin.ErrorResponse(err)
	}

	if !setupResp.Success {
		return plugin.ErrorResponseMsg(setupResp.Error)
	}

	marker, err := hyperliquid.BuildStreamMarker(setupResp, streamReq.StreamID)
	if err != nil {
		return plugin.ErrorResponseMsg(err.Error())
	}

	if err := marker.Validate(); err != nil {
		return plugin.ErrorResponseMsg(err.Error())
	}

	return plugin.SuccessTypedResponse("StreamMarker", marker)
}


func (p *HyperliquidPlugin) handleGetAccount(_ map[string]any) plugin.Response {
	if p.client == nil {
		return plugin.ErrorResponseMsg("client not initialized")
	}

	account, err := p.client.GetAccount()
	if err != nil {
		return plugin.ErrorResponse(err)
	}

	return plugin.SuccessResponse(account, time.Second*5)
}


func (p *HyperliquidPlugin) handleGetBalances(_ map[string]any) plugin.Response {
	if p.client == nil {
		return plugin.ErrorResponseMsg("client not initialized")
	}

	balances, err := p.client.GetBalances()
	if err != nil {
		return plugin.ErrorResponse(err)
	}

	return plugin.SuccessResponse(balances, time.Second*3)
}


func (p *HyperliquidPlugin) handleGetPositions(_ map[string]any) plugin.Response {
	if p.client == nil {
		return plugin.ErrorResponseMsg("client not initialized")
	}

	positions, err := p.client.GetPositions()
	if err != nil {
		return plugin.ErrorResponse(err)
	}

	return plugin.SuccessResponse(positions, time.Second*3)
}


func (p *HyperliquidPlugin) handleGetOrders(params map[string]any) plugin.Response {
	if p.client == nil {
		return plugin.ErrorResponseMsg("client not initialized")
	}

	orderParams := ex.GetOrdersParamsFromMap(params)

	orders, err := p.client.GetOrders(orderParams.OpenOnly)
	if err != nil {
		return plugin.ErrorResponse(err)
	}

	return plugin.SuccessResponse(orders, time.Second*3)
}


func (p *HyperliquidPlugin) handleCreateOrders(params map[string]any) plugin.Response {
	if p.client == nil {
		return plugin.ErrorResponseMsg("client not initialized")
	}

	parsed := ex.CreateOrdersParamsFromMap(params)
	if err := parsed.Validate(); err != nil {
		return plugin.ErrorResponse(err)
	}

	resp, err := p.client.CreateOrders(parsed)
	if err != nil {
		return plugin.ErrorResponse(err)
	}

	return plugin.SuccessResponse(resp)
}


func (p *HyperliquidPlugin) handleCancelOrders(params map[string]any) plugin.Response {
	if p.client == nil {
		return plugin.ErrorResponseMsg("client not initialized")
	}

	parsed := ex.CancelOrdersParamsFromMap(params)

	resp, err := p.client.CancelOrders(parsed)
	if err != nil {
		return plugin.ErrorResponse(err)
	}

	return plugin.SuccessResponse(resp)
}
