# Hyperliquid Datasource Plugin

A Terminal datasrc plugin for the Hyperliquid L1 exchange, supporting
both perpetual and spot markets with agent wallet (API wallet) trading.

## Features

- **Market data**: perp + spot market discovery, candle/OHLCV history (5000 most recent candles),
  real-time candle streaming via WebSocket
- **Account data**: clearinghouse state (positions, margin, equity), spot balances, open orders
- **Trading**: limit/market/trigger (TP/SL) orders, batch orders, cancel by OID or cloid, TWAP orders
- **Authentication**: agent wallet (API wallet) signing with pure-Go secp256k1 + EIP-712

## Setup

### Prerequisites

1. A Hyperliquid master account with USDC deposited
2. An agent (API) wallet approved by the master account

### Configuration Fields

| Field | Description | Required | Encrypted |
|-------|-------------|----------|-----------|
| walletAddress | Master or sub-account address (0x...) | Yes | Yes |
| privateKey | Agent wallet private key (64 hex chars) | Yes | Yes |
| testnet | Connect to testnet | No | No |

The walletAddress is the master/sub-account that the agent is approved for.
The privateKey is the agent's key used for signing exchange actions.

## Build

    ./build.sh

## License

MIT
