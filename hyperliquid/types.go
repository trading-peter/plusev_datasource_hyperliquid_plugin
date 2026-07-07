package hyperliquid

import "encoding/json"


// ============================================================================
// Info Endpoint Types (REST POST /info)
// ============================================================================


// InfoRequest is the generic body for all /info POST requests.
type InfoRequest struct {
	Type     string         `json:"type"`
	User     string         `json:"user,omitempty"`
	Coin     string         `json:"coin,omitempty"`
	Dex      string         `json:"dex,omitempty"`
	Req      map[string]any `json:"req,omitempty"`
	StartTime *int64        `json:"startTime,omitempty"`
	EndTime   *int64        `json:"endTime,omitempty"`
}


// MetaResponse is the response from "meta" — perpetual universe + margin tables.
type MetaResponse struct {
	Universe     []PerpAsset     `json:"universe"`
	MarginTables  [][]any         `json:"marginTables,omitempty"`
	CollateralToken int           `json:"collateralToken,omitempty"`
}


// MetaAndAssetCtxsResponse is the combined response from "metaAndAssetCtxs".
type MetaAndAssetCtxsResponse struct {
	Meta      MetaResponse
	AssetCtxs []PerpAssetCtx
}


// PerpAsset represents one perpetual in the universe.
type PerpAsset struct {
	Name          string `json:"name"`
	SzDecimals    int    `json:"szDecimals"`
	MaxLeverage   int    `json:"maxLeverage"`
	OnlyIsolated  bool   `json:"onlyIsolated,omitempty"`
	MarginMode    string `json:"marginMode,omitempty"`
	IsDelisted    bool   `json:"isDelisted,omitempty"`
}


// PerpAssetCtx contains market context for one perpetual.
type PerpAssetCtx struct {
	Funding      string `json:"funding"`
	MarkPx       string `json:"markPx"`
	MidPx        string `json:"midPx"`
	OraclePx     string `json:"oraclePx"`
	OpenInterest string `json:"openInterest"`
	PrevDayPx    string `json:"prevDayPx"`
	DayNtlVlm    string `json:"dayNtlVlm"`
}


// SpotMetaResponse is the response from "spotMeta".
type SpotMetaResponse struct {
	Tokens   []SpotToken  `json:"tokens"`
	Universe []SpotPair   `json:"universe"`
}


// SpotMetaAndAssetCtxsResponse combines spotMeta with spot asset contexts.
type SpotMetaAndAssetCtxsResponse struct {
	Meta      SpotMetaResponse
	AssetCtxs []SpotAssetCtx
}


// SpotToken represents a token in the spot system.
type SpotToken struct {
	Name         string `json:"name"`
	SzDecimals   int    `json:"szDecimals"`
	WeiDecimals  int    `json:"weiDecimals"`
	Index        int    `json:"index"`
	TokenID      string `json:"tokenId"`
	IsCanonical  bool   `json:"isCanonical"`
	EvmContract  *string `json:"evmContract"`
	FullName     *string `json:"fullName"`
}


// SpotPair represents one spot trading pair.
type SpotPair struct {
	Name        string `json:"name"`
	Tokens      []int  `json:"tokens"`
	Index       int    `json:"index"`
	IsCanonical bool   `json:"isCanonical"`
}


// SpotAssetCtx contains market context for one spot pair.
type SpotAssetCtx struct {
	DayNtlVlm string `json:"dayNtlVlm"`
	MarkPx    string `json:"markPx"`
	MidPx     string `json:"midPx"`
	PrevDayPx string `json:"prevDayPx"`
}


// CandleSnapshot represents a single OHLCV candle from the API.
type CandleSnapshot struct {
	T int64  `json:"t"` // open time (ms)
	C string `json:"c"` // close
	H string `json:"h"` // high
	I string `json:"i"` // interval
	L string `json:"l"` // low
	N int    `json:"n"` // number of trades
	O string `json:"o"` // open
	S string `json:"s"` // symbol
	V string `json:"v"` // volume
}


// ClearinghouseStateResponse is the response from "clearinghouseState".
type ClearinghouseStateResponse struct {
	AssetPositions             []AssetPosition `json:"assetPositions"`
	CrossMaintenanceMarginUsed string          `json:"crossMaintenanceMarginUsed"`
	CrossMarginSummary         MarginSummary   `json:"crossMarginSummary"`
	MarginSummary              MarginSummary   `json:"marginSummary"`
	Time                       int64           `json:"time"`
	Withdrawable               string          `json:"withdrawable"`
}


// AssetPosition wraps a position with its type.
type AssetPosition struct {
	Position PositionDetail `json:"position"`
	Type     string         `json:"type"`
}


// PositionDetail contains per-coin position details.
type PositionDetail struct {
	Coin           string        `json:"coin"`
	EntryPx        string        `json:"entryPx"`
	Leverage       PositionLev   `json:"leverage"`
	LiquidationPx  string        `json:"liquidationPx"`
	MarginUsed     string        `json:"marginUsed"`
	MaxLeverage    int           `json:"maxLeverage"`
	PositionValue  string        `json:"positionValue"`
	ReturnOnEquity string        `json:"returnOnEquity"`
	Szi            string        `json:"szi"`
	UnrealizedPnl  string        `json:"unrealizedPnl"`
	CumFunding     *CumFunding   `json:"cumFunding,omitempty"`
}


// PositionLev describes the leverage type and value.
type PositionLev struct {
	RawUSD string `json:"rawUsd,omitempty"`
	Type   string `json:"type"`   // "cross" or "isolated"
	Value  int    `json:"value"`  // leverage multiplier
}


// CumFunding holds cumulative funding data.
type CumFunding struct {
	AllTime      string `json:"allTime"`
	SinceChange  string `json:"sinceChange"`
	SinceOpen    string `json:"sinceOpen"`
}


// MarginSummary holds aggregate margin information.
type MarginSummary struct {
	AccountValue   string `json:"accountValue"`
	TotalMarginUsed string `json:"totalMarginUsed"`
	TotalNtlPos    string `json:"totalNtlPos"`
	TotalRawUsd    string `json:"totalRawUsd"`
}


// SpotClearinghouseStateResponse is the response from "spotClearinghouseState".
type SpotClearinghouseStateResponse struct {
	Balances []SpotBalance `json:"balances"`
}


// SpotBalance represents one token balance in the spot account.
type SpotBalance struct {
	Coin    string `json:"coin"`
	Token   int    `json:"token"`
	Hold    string `json:"hold"`
	Total   string `json:"total"`
	EntryNtl string `json:"entryNtl"`
}


// FrontendOpenOrder represents one open order from "frontendOpenOrders".
type FrontendOpenOrder struct {
	Coin             string `json:"coin"`
	Side             string `json:"side"` // "B" = buy, "A" = sell
	LimitPx          string `json:"limitPx"`
	Oid              int64  `json:"oid"`
	Sz               string `json:"sz"`
	OrigSz           string `json:"origSz"`
	OrderType        string `json:"orderType"`
	ReduceOnly       bool   `json:"reduceOnly"`
	Timestamp        int64  `json:"timestamp"`
	IsTrigger        bool   `json:"isTrigger"`
	TriggerCondition string `json:"triggerCondition"`
	TriggerPx        string `json:"triggerPx"`
	IsPositionTpsl   bool   `json:"isPositionTpsl"`
	Cloid            *string `json:"cloid"`
}


// OrderStatusResponse wraps the response from "orderStatus".
type OrderStatusResponse struct {
	Status         string          `json:"status"`
	Order          json.RawMessage `json:"order,omitempty"`
	StatusTimestamp int64          `json:"statusTimestamp,omitempty"`
}


// ============================================================================
// Exchange Endpoint Types (REST POST /exchange)
// ============================================================================


// ExchangeResponse is the generic response wrapper from the /exchange endpoint.
type ExchangeResponse struct {
	Status   string          `json:"status"`
	Response json.RawMessage `json:"response"`
}


// OrderActionResponse is the typed inner response for order actions.
type OrderActionResponse struct {
	Type     string        `json:"type"`
	Data     OrderResultData `json:"data"`
}


// OrderResultData contains the per-order statuses.
type OrderResultData struct {
	Statuses []OrderStatus `json:"statuses"`
}


// OrderStatus represents the result of one order in a batch.
type OrderStatus struct {
	Resting *OrderResting `json:"resting,omitempty"`
	Filled  *OrderFilled  `json:"filled,omitempty"`
	Error   *string       `json:"error,omitempty"`
}


// OrderResting indicates the order was placed and is resting on the book.
type OrderResting struct {
	Oid int64 `json:"oid"`
}


// OrderFilled indicates the order was immediately filled.
type OrderFilled struct {
	TotalSz string `json:"totalSz"`
	AvgPx   string `json:"avgPx"`
	Oid     int64  `json:"oid"`
}


// DefaultResponse is the generic "ok" response for non-order actions.
type DefaultResponse struct {
	Type string `json:"type"`
}


// ============================================================================
// Msgpack Action Structs (for L1 action signing — field order is critical!)
// ============================================================================


// OrderWireType encodes the order type for the wire format.
type OrderWireType struct {
	Limit   *OrderWireLimit   `msgpack:"limit,omitempty"`
	Trigger *OrderWireTrigger `msgpack:"trigger,omitempty"`
}


// OrderWireLimit encodes a limit order type.
type OrderWireLimit struct {
	Tif string `msgpack:"tif"`
}


// OrderWireTrigger encodes a trigger (TP/SL) order type.
type OrderWireTrigger struct {
	IsMarket  bool   `msgpack:"isMarket"`
	TriggerPx string `msgpack:"triggerPx"`
	Tpsl      string `msgpack:"tpsl"`
}


// OrderWire is the wire format for a single order.
// Field order MUST match Python SDK: a, b, p, s, r, t, c.
type OrderWire struct {
	Asset      int           `msgpack:"a"`
	IsBuy      bool          `msgpack:"b"`
	LimitPx    string        `msgpack:"p"`
	Size       string        `msgpack:"s"`
	ReduceOnly bool          `msgpack:"r"`
	OrderType  OrderWireType `msgpack:"t"`
	Cloid      *string       `msgpack:"c,omitempty"`
}


// OrderAction is the L1 action for placing orders.
// Field order MUST match Python SDK: type, orders, grouping.
type OrderAction struct {
	Type     string      `msgpack:"type"`
	Orders   []OrderWire `msgpack:"orders"`
	Grouping string      `msgpack:"grouping"`
}


// CancelWire is one cancel request item (by OID).
type CancelWire struct {
	Asset   int   `msgpack:"a"`
	OrderID int64 `msgpack:"o"`
}


// CancelAction is the L1 action for cancelling orders by OID.
type CancelAction struct {
	Type    string       `msgpack:"type"`
	Cancels []CancelWire `msgpack:"cancels"`
}


// CancelByCloidWire is one cancel request item (by client order ID).
// Note: uses "asset" and "cloid" keys (not "a" and "o" like CancelWire).
type CancelByCloidWire struct {
	Asset int    `msgpack:"asset"`
	Cloid string `msgpack:"cloid"`
}


// CancelByCloidAction is the L1 action for cancelling orders by cloid.
type CancelByCloidAction struct {
	Type    string              `msgpack:"type"`
	Cancels []CancelByCloidWire `msgpack:"cancels"`
}


// TwapOrderWire is the wire format for a TWAP order.
type TwapOrderWire struct {
	Asset      int    `msgpack:"a"`
	IsBuy      bool   `msgpack:"b"`
	Size       string `msgpack:"s"`
	ReduceOnly bool   `msgpack:"r"`
	Minutes    int    `msgpack:"m"`
	Randomize  bool   `msgpack:"t"`
}


// TwapOrderAction is the L1 action for placing a TWAP order.
type TwapOrderAction struct {
	Type string        `msgpack:"type"`
	Twap TwapOrderWire `msgpack:"twap"`
}


// TwapCancelAction is the L1 action for cancelling a TWAP order.
type TwapCancelAction struct {
	Type   string `msgpack:"type"`
	Asset  int    `msgpack:"a"`
	TwapID int64  `msgpack:"t"`
}
