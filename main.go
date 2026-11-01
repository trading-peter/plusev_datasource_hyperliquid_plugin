package main

import (
	"strings"
	"time"

	"github.com/plusev-terminal/go-plugin-common/v2/plugin"
	"github.com/plusev-terminal/go-plugin-common/v2/requester"

	"github.com/trading-peter/plusev_datasource_hyperliquid_plugin/hyperliquid"

	ex "github.com/plusev-terminal/go-plugin-common/v2/datasrc/exchange"
	acct "github.com/plusev-terminal/go-plugin-common/v2/datasrc/exchange/account"
	m "github.com/plusev-terminal/go-plugin-common/v2/meta"
	tt "github.com/plusev-terminal/go-plugin-common/v2/trading"
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

// tradingReserve keeps part of each API host's weight budget free for order
// commands (createOrders/cancelOrders), which share the host with data pulls.
var tradingReserve = 0.3

func (p *HyperliquidPlugin) GetMeta() m.Meta {
	return m.Meta{
		PluginID:    "hyperliquid-datasource",
		Name:        "Hyperliquid",
		AppID:       "datasrc",
		Category:    "dex",
		Description: "Hyperliquid L1 exchange — perps and spot with agent wallet trading",
		Author:      "trading_peter",
		Version: "0.2.0",
		Repository:  "https://github.com/trading-peter/plusev_datasource_hyperliquid_plugin",
		Tags:        []string{"hyperliquid", "dex", "perps", "spot", "l1"},
		Contacts: []m.AuthorContact{
			{Kind: "x.com", Value: "https://x.com/trading_peter"},
		},
		Resources: m.ResourceAccess{
			AllowedNetworkTargets: []m.NetworkTargetRule{
				// Hyperliquid rate limits, retrieved 2026-10-31:
				// https://hyperliquid.gitbook.io/hyperliquid-docs/for-developers/api/rate-limits-and-user-limits
				// "REST requests share an aggregated weight limit of 1200 per
				// minute" per IP. Info request weights: 2 (l2Book, allMids,
				// clearinghouseState, orderStatus, spotClearinghouseState,
				// exchangeStatus), 60 (userRole), 20 (all other documented info
				// requests). Exchange actions weigh 1 + floor(batch_length/40).
				// The per-request Cost is set in hyperliquid/client.go.
				{
					Pattern: "https://api.hyperliquid.xyz/*",
					Limits: []m.RateLimitRule{
						// 1200 weight/min per IP shared by /info and /exchange
						// (= 20 weight/s); the docs publish a 1-minute window
						// and no smaller bucket, so burst = the full window.
						m.NewRateLimitRule("weight", 1200, time.Minute).WithLabel("Request weight").Weighted(),
					},
					TradingReserve: &tradingReserve,
				},
				{
					// Same published budget as mainnet; order actions share
					// the host with data pulls here too.
					Pattern: "https://api.hyperliquid-testnet.xyz/*",
					Limits: []m.RateLimitRule{
						m.NewRateLimitRule("weight", 1200, time.Minute).WithLabel("Request weight").Weighted(),
					},
					TradingReserve: &tradingReserve,
				},
				// WebSocket targets stay unlimited: the documented WS caps
				// (10 connections, 2000 msgs/min, ...) are connection-level,
				// not HTTP request budgets.
				{Pattern: "wss://api.hyperliquid.xyz/*"},
				{Pattern: "wss://api.hyperliquid-testnet.xyz/*"},
			},
		},
		Features: []string{},
	}
}

// GetRateLimits intentionally returns an empty slice: Hyperliquid's real
// budget (1200 weight/min per IP) is declared as manifest Limits on the
// network targets above, and returning per-command values here would stack
// the legacy limiter on top of it.
func (p *HyperliquidPlugin) GetRateLimits() []plugin.RateLimit {
	return []plugin.RateLimit{}
}

// GetOrderCapabilities declares the order surface this plugin really
// implements (see hyperliquid/createOrders.go). Hyperliquid serves spot and
// perpetual markets; TP/SL orders always trigger on the mark price — the
// exchange API has no trigger-source selector — and the venue has neither
// trailing stops nor order expiry, GTD or otherwise, so those stay
// undeclared and the host rejects them upfront.
func (p *HyperliquidPlugin) GetOrderCapabilities() ex.OrderCapabilities {
	return ex.OrderCapabilities{
		AssetTypes: []tt.AssetType{tt.AssetTypeSpot, tt.AssetTypePerpetual},
		OrderTypes: []acct.OrderType{
			acct.OrderTypeLimit,
			acct.OrderTypeMarket,
			acct.OrderTypeStop,
			acct.OrderTypeStopLimit,
			acct.OrderTypeStopMarket,
			acct.OrderTypeTakeProfit,
			acct.OrderTypeTakeProfitLimit,
			acct.OrderTypeTakeProfitMarket,
		},
		TimeInForce: []acct.TimeInForce{
			acct.TimeInForceGTC, // wire "Gtc" (default)
			acct.TimeInForceIOC, // wire "Ioc"
			acct.TimeInForcePostOnly,
			acct.TimeInForceGTX, // both post-only variants map to wire "Alo"
		},
		TriggerSources:       []acct.TriggerSource{acct.TriggerSourceMark},
		DefaultTriggerSource: acct.TriggerSourceMark,
		ReduceOnly:           true,
		PostOnly:             true,
		QuoteQuantity:        true,
		ClientOrderID:        true,
		// Extra keys read when routing/placing orders (TWAP routing and its
		// two knobs); every other Extra key is rejected by the host.
		ExtraKeys: []string{"twap", "minutes", "randomize"},
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
