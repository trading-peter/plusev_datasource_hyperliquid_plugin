package hyperliquid

import (
	"strings"
	"time"

	acct "github.com/plusev-terminal/go-plugin-common/v2/datasrc/exchange/account"
)

// GetBalances returns balances only (no account metadata beyond balances snapshot).
func (c *Client) GetBalances() (acct.BalancesResponse, error) {
	clearinghouse, err := c.getClearinghouseState()
	if err != nil {
		return acct.BalancesResponse{}, err
	}

	spot, err := c.getSpotClearinghouseState()
	if err != nil {
		c.log.ErrorWithData("Failed to fetch spot balances", map[string]any{"error": err})
		spot = &SpotClearinghouseStateResponse{}
	}

	fetchedAt := time.Now()
	if clearinghouse.Time > 0 {
		fetchedAt = time.UnixMilli(clearinghouse.Time)
	}

	resp := acct.BalancesResponse{
		FetchedAt: fetchedAt,
		Scopes:    map[acct.ScopeType]acct.BalanceScope{},
		Extra:     map[string]any{},
	}

	// Futures scope — margin/equity summary
	futuresScope := acct.BalanceScope{
		Balances: map[string]acct.AssetBalance{},
		State: &acct.ScopeState{
			Equity:          clearinghouse.MarginSummary.AccountValue,
			AvailableMargin: clearinghouse.Withdrawable,
			UsedMargin:      clearinghouse.MarginSummary.TotalMarginUsed,
		},
		Extra: map[string]any{
			"total_ntl_pos": clearinghouse.MarginSummary.TotalNtlPos,
		},
	}

	// USDC as the futures collateral
	futuresScope.Balances["USDC"] = acct.AssetBalance{
		Asset:               "USDC",
		Total:               clearinghouse.MarginSummary.AccountValue,
		AvailableToTrade:    clearinghouse.Withdrawable,
		AvailableToWithdraw: clearinghouse.Withdrawable,
		Components: map[string]string{
			"margin_used": clearinghouse.MarginSummary.TotalMarginUsed,
		},
	}

	resp.Scopes[acct.ScopeFutures] = futuresScope

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
			available := bal.Total

			if bal.Hold != "" {
				available = subtractStr(bal.Total, bal.Hold)
			}

			spotScope.Balances[asset] = acct.AssetBalance{
				Asset:            asset,
				Total:            bal.Total,
				AvailableToTrade: available,
				Components: map[string]string{
					"hold": bal.Hold,
				},
			}
		}

		resp.Scopes[acct.ScopeSpot] = spotScope
	}

	return resp, nil
}

// subtractStr performs string-based float subtraction for simple values.
func subtractStr(a, b string) string {
	af := parseFloatStr(a)
	bf := parseFloatStr(b)

	return formatFloat(af - bf)
}
