package hyperliquid

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/plusev-terminal/go-plugin-common/v2/logging"
	"github.com/plusev-terminal/go-plugin-common/v2/plugin"
	rt "github.com/plusev-terminal/go-plugin-common/v2/requester/types"
)

// Client is the Hyperliquid API client.
type Client struct {
	name      string
	baseURL   string
	wsBaseURL string
	requester rt.RequestDoer
	log       *logging.Logger

	isMainnet bool

	// Agent wallet credentials
	walletAddress string
	privKeyBytes  []byte

	// Asset index caches (populated on first use)
	perpIndexCache map[string]int // coin name → universe index
	spotIndexCache map[string]int // pair name → 10000+index
}

// NewClient creates a new Hyperliquid API client.
func NewClient(req rt.RequestDoer, baseURL string, isMainnet bool) *Client {
	wsURL := "wss://api.hyperliquid.xyz/ws"
	if !isMainnet {
		wsURL = "wss://api.hyperliquid-testnet.xyz/ws"
	}

	return &Client{
		name:           "Hyperliquid",
		baseURL:        baseURL,
		wsBaseURL:      wsURL,
		requester:      req,
		log:            logging.NewLogger("hyperliquid-datasource"),
		isMainnet:      isMainnet,
		perpIndexCache: nil,
		spotIndexCache: nil,
	}
}

// SetCredentials stores the decrypted wallet credentials.
func (c *Client) SetCredentials(creds map[string]string) {
	if addr, ok := creds["walletAddress"]; ok {
		c.walletAddress = strings.TrimSpace(addr)
	}

	if pk, ok := creds["privateKey"]; ok {
		pkBytes, err := PrivateKeyBytes(pk)
		if err != nil {
			c.log.ErrorWithData("Failed to parse private key", map[string]any{"error": err})
			return
		}

		c.privKeyBytes = pkBytes

		// If wallet address is not set, derive it from the private key.
		// This handles the agent wallet case where the user only provides the key.
		if c.walletAddress == "" {
			c.walletAddress = DeriveAddress(pkBytes)
		}
	}
}

func (c *Client) GetName() string {
	return c.name
}

func (c *Client) GetConfigFields() []plugin.ConfigField {
	return []plugin.ConfigField{
		{
			Label:       "Wallet Address",
			Name:        "walletAddress",
			Description: "Your Hyperliquid wallet address (0x...). This is the master or sub-account address that the agent wallet is approved for.",
			Required:    true,
			Encrypt:     true,
			Mask:        false,
		},
		{
			Label:       "Private Key",
			Name:        "privateKey",
			Description: "Your agent (API) wallet private key. The agent must be approved by the master account.",
			Required:    true,
			Encrypt:     true,
			Mask:        true,
		},
		{
			Label:       "Use Testnet",
			Name:        "testnet",
			Description: "Connect to Hyperliquid testnet instead of mainnet.",
			Required:    false,
			Type:        "boolean",
		},
	}
}

// infoWeight returns the documented /info request weight used as the
// Cost for the host's weighted rate-limit rule: 2 for l2Book, allMids,
// clearinghouseState, orderStatus, spotClearinghouseState and exchangeStatus;
// 60 for userRole; 20 for all other documented info requests.
// candleSnapshot additionally scales with response size (weight per 60 items
// returned), which cannot be known before the request — the documented base
// weight is charged.
// Source: https://hyperliquid.gitbook.io/hyperliquid-docs/for-developers/api/rate-limits-and-user-limits
func infoWeight(infoType string) int {
	switch infoType {
	case "l2Book", "allMids", "clearinghouseState", "orderStatus", "spotClearinghouseState", "exchangeStatus":
		return 2
	case "userRole":
		return 60
	default:
		return 20
	}
}

// exchangeWeight returns the documented /exchange action weight
// 1 + floor(batch_length/40), where batch_length is the number of orders or
// cancels in the action; unbatched actions (e.g. twapOrder) weigh 1.
// Source: https://hyperliquid.gitbook.io/hyperliquid-docs/for-developers/api/rate-limits-and-user-limits
func exchangeWeight(action any) int {
	batchLen := 0
	switch a := action.(type) {
	case OrderAction:
		batchLen = len(a.Orders)
	case CancelAction:
		batchLen = len(a.Cancels)
	case CancelByCloidAction:
		batchLen = len(a.Cancels)
	}
	return 1 + batchLen/40
}

// post sends a POST request to an API endpoint and decodes the response.
// cost is the request's weight for the host's weighted rate limit; 0 counts
// as 1.
func (c *Client) post(path string, body any, result any, cost int) error {
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("failed to marshal request body: %w", err)
	}

	req := &rt.Request{
		Method:  "POST",
		URL:     c.baseURL + path,
		Headers: map[string]string{"Content-Type": "application/json"},
		Body:    bodyBytes,
		Cost:    cost,
	}

	_, err = c.requester.Send(req, result)
	if err != nil {
		return err
	}

	return nil
}

// postInfo sends a request to the /info endpoint.
func (c *Client) postInfo(infoReq InfoRequest, result any) error {
	return c.post("/info", infoReq, result, infoWeight(infoReq.Type))
}

// postExchange sends a signed action to the /exchange endpoint.
func (c *Client) postExchange(action any, result any) error {
	nonce := currentTimestampMs()

	sig, err := SignL1Action(c.privKeyBytes, action, "", nonce, nil, c.isMainnet)
	if err != nil {
		return fmt.Errorf("failed to sign action: %w", err)
	}

	body := map[string]any{
		"action":    action,
		"nonce":     nonce,
		"signature": sig,
	}

	return c.post("/exchange", body, result, exchangeWeight(action))
}

// ensureAssetIndex loads perp and spot metadata if not cached.
func (c *Client) ensureAssetIndex() error {
	if c.perpIndexCache != nil {
		return nil
	}

	// Fetch perp metadata
	var metaRaw json.RawMessage
	if err := c.postInfo(InfoRequest{Type: "metaAndAssetCtxs"}, &metaRaw); err != nil {
		return fmt.Errorf("failed to fetch perp metadata: %w", err)
	}

	var metaArr []json.RawMessage
	if err := json.Unmarshal(metaRaw, &metaArr); err != nil {
		return fmt.Errorf("failed to parse metaAndAssetCtxs envelope: %w", err)
	}

	if len(metaArr) < 1 {
		return fmt.Errorf("metaAndAssetCtxs returned empty array")
	}

	var meta MetaResponse
	if err := json.Unmarshal(metaArr[0], &meta); err != nil {
		return fmt.Errorf("failed to parse perp meta: %w", err)
	}

	c.perpIndexCache = make(map[string]int, len(meta.Universe))
	for i, asset := range meta.Universe {
		c.perpIndexCache[asset.Name] = i
	}

	// Fetch spot metadata
	var spotRaw json.RawMessage
	if err := c.postInfo(InfoRequest{Type: "spotMetaAndAssetCtxs"}, &spotRaw); err != nil {
		// Spot metadata might not always be available; continue without it
		c.spotIndexCache = make(map[string]int)
		return nil
	}

	var spotArr []json.RawMessage
	if err := json.Unmarshal(spotRaw, &spotArr); err != nil {
		c.spotIndexCache = make(map[string]int)
		return nil
	}

	if len(spotArr) < 1 {
		c.spotIndexCache = make(map[string]int)
		return nil
	}

	var spotMeta SpotMetaResponse
	if err := json.Unmarshal(spotArr[0], &spotMeta); err != nil {
		c.spotIndexCache = make(map[string]int)
		return nil
	}

	c.spotIndexCache = make(map[string]int, len(spotMeta.Universe))
	for _, pair := range spotMeta.Universe {
		c.spotIndexCache[pair.Name] = 10000 + pair.Index
	}

	return nil
}

// coinToAssetIndex converts a symbol/coin name to the Hyperliquid asset index.
// For perps: returns the universe index directly.
// For spot: returns 10000 + index.
func (c *Client) coinToAssetIndex(symbol string, isSpot bool) (int, error) {
	if err := c.ensureAssetIndex(); err != nil {
		return 0, err
	}

	if isSpot {
		// Try exact pair name first (e.g., "PURR/USDC")
		if idx, ok := c.spotIndexCache[symbol]; ok {
			return idx, nil
		}

		// Try @index format
		if strings.HasPrefix(symbol, "@") {
			if idx, ok := c.spotIndexCache[symbol]; ok {
				return idx, nil
			}
		}

		return 0, fmt.Errorf("spot pair %s not found in spot metadata", symbol)
	}

	if idx, ok := c.perpIndexCache[symbol]; ok {
		return idx, nil
	}

	return 0, fmt.Errorf("coin %s not found in perp metadata", symbol)
}

// normalizeCloid ensures a cloid string is a valid 0x-prefixed 128-bit hex.
// If the input is already valid hex, it is returned as-is.
// Otherwise, it is hashed to 16 bytes.
func normalizeCloid(cloid string) string {
	cloid = strings.TrimSpace(cloid)
	if cloid == "" {
		return ""
	}

	if !strings.HasPrefix(cloid, "0x") {
		cloid = "0x" + cloid
	}

	hexPart := cloid[2:]
	if len(hexPart) == 32 {
		return cloid
	}

	// If not 32 hex chars, hash the original value to produce a valid cloid
	return "0x" + fmt.Sprintf("%x", keccak256([]byte(cloid))[:16])
}
