package hyperliquid

import (
	"fmt"
	"strings"
	"time"

	acct "github.com/plusev-terminal/go-plugin-common/v2/datasrc/exchange/account"
)

// GetAccount fetches the user's perpetuals account summary and spot balances.
func (c *Client) GetAccount() (acct.Account, error) {
	clearinghouse, err := c.getClearinghouseState()
	if err != nil {
		return acct.Account{}, err
	}

	spot, err := c.getSpotClearinghouseState()
	if err != nil {
		c.log.ErrorWithData("Failed to fetch spot state for account", map[string]any{"error": err})
		spot = &SpotClearinghouseStateResponse{}
	}

	fetchedAt := time.Now()
	if clearinghouse.Time > 0 {
		fetchedAt = time.UnixMilli(clearinghouse.Time)
	}

	account := acct.Account{
		Exchange:     "hyperliquid",
		AccountID:    c.walletAddress,
		Type:         acct.AccountTypeFutures,
		FetchedAt:    fetchedAt,
		CustodyModel: acct.CustodyModelSelfCustody,
		IsCustodial:  true,
		Scopes:       map[acct.ScopeType]acct.BalanceScope{},
		Extra:        map[string]any{},
	}

	// Futures scope — margin summary
	futuresScope := acct.BalanceScope{
		Balances: map[string]acct.AssetBalance{},
		State: &acct.ScopeState{
			Equity:          clearinghouse.MarginSummary.AccountValue,
			AvailableMargin: clearinghouse.Withdrawable,
			UsedMargin:      clearinghouse.MarginSummary.TotalMarginUsed,
		},
		Extra: map[string]any{
			"total_ntl_pos":       clearinghouse.MarginSummary.TotalNtlPos,
			"cross_margin_used":   clearinghouse.CrossMarginSummary.TotalMarginUsed,
			"cross_account_value": clearinghouse.CrossMarginSummary.AccountValue,
		},
	}
	account.Scopes[acct.ScopeFutures] = futuresScope

	// Spot scope — token balances
	if spot != nil && len(spot.Balances) > 0 {
		spotScope := acct.BalanceScope{
			Balances: map[string]acct.AssetBalance{},
			Extra:    map[string]any{},
		}

		for _, bal := range spot.Balances {
			if strings.TrimSpace(bal.Total) == "" || bal.Total == "0" {
				continue
			}

			asset := strings.ToUpper(bal.Coin)
			spotScope.Balances[asset] = acct.AssetBalance{
				Asset:            asset,
				Total:            bal.Total,
				AvailableToTrade: bal.Total,
				Components: map[string]string{
					"hold": bal.Hold,
				},
				Extra: map[string]any{
					"entry_ntl":   bal.EntryNtl,
					"token_index": bal.Token,
				},
			}
		}

		account.Scopes[acct.ScopeSpot] = spotScope
	}

	// Max leverage from positions
	maxLev := 0
	for _, pos := range clearinghouse.AssetPositions {
		if pos.Position.MaxLeverage > maxLev {
			maxLev = pos.Position.MaxLeverage
		}
	}
	account.MaxLeverage = maxLev

	return account, nil
}

// getClearinghouseState fetches the perpetuals clearinghouse state.
func (c *Client) getClearinghouseState() (*ClearinghouseStateResponse, error) {
	var result ClearinghouseStateResponse

	err := c.postInfo(InfoRequest{Type: "clearinghouseState", User: c.walletAddress}, &result)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch clearinghouse state: %w", err)
	}

	return &result, nil
}

// getSpotClearinghouseState fetches spot token balances.
func (c *Client) getSpotClearinghouseState() (*SpotClearinghouseStateResponse, error) {
	var result SpotClearinghouseStateResponse

	err := c.postInfo(InfoRequest{Type: "spotClearinghouseState", User: c.walletAddress}, &result)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch spot clearinghouse state: %w", err)
	}

	return &result, nil
}
