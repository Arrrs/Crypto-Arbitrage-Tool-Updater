package models

import "time"

type Pair struct {
	PairKey               string    `json:"key"`      // Composite key: symbol_exchange_market (e.g., "BTCUSDT_Binance_spot")
	Symbol                string    `json:"symbol"`   // Trading symbol (e.g., "BTCUSDT")
	Exchange              string    `json:"exchange"` // Market exchange (e.g., "Binance")
	Market                string    `json:"market"`   // Market type (e.g., "spot" or "futures")
	Price                 float64   `json:"price"`
	BaseAsset             string    `json:"baseAsset"`   // Base asset (e.g., "BTC")
	QuoteAsset            string    `json:"quoteAsset"`  // Quote asset (e.g., "USDT")
	DisplayName           string    `json:"displayName"` // Formatted display (e.g., "BTC/USDT")
	PriceChangePercent24h float64   `json:"priceChangePercent24h"`
	BaseVolume24h         float64   `json:"baseVolume24h"`
	QuoteVolume24h        float64   `json:"quoteVolume24h"`
	UpdatedAt             time.Time `json:"updated_at"`
	CreatedAt             time.Time `json:"created_at"`
}

type PairFutures struct {
	PairKey               string    `json:"key"`      // Composite key: symbol_exchange_market (e.g., "BTCUSDT_Binance_spot")
	Symbol                string    `json:"symbol"`   // Trading symbol (e.g., "BTCUSDT")
	Exchange              string    `json:"exchange"` // Market exchange (e.g., "Binance")
	Market                string    `json:"market"`   // Market type (e.g., "spot" or "futures")
	MarkPrice             float64   `json:"markprice"`
	IndexPrice            float64   `json:"indexprice"`
	BaseAsset             string    `json:"baseAsset"`   // Base asset (e.g., "BTC")
	QuoteAsset            string    `json:"quoteAsset"`  // Quote asset (e.g., "USDT")
	DisplayName           string    `json:"displayName"` // Formatted display (e.g., "BTC/USDT")
	FundingRatePercent    float64   `json:"fundingRatePercent"`
	NextFundingTimestamp  int       `json:"nextFundingTimestamp"`
	PriceChangePercent24h float64   `json:"priceChangePercent24h"`
	BaseVolume24h         float64   `json:"baseVolume24h"`
	QuoteVolume24h        float64   `json:"quoteVolume24h"`
	UpdatedAt             time.Time `json:"updated_at"`
	CreatedAt             time.Time `json:"created_at"`
}

// Example Pair usage:
// {
//   key: "BTCUSDT_Binance_spot",
//   symbol: "BTCUSDT",
//   exchange: "Binance",
//   market: "spot",
//   price: 912345.67,
//   baseAsset: "BTC",
//   quoteAsset: "USDT",
//   displayName: "BTC/USDT",
//   priceChangePercent24h: -1.23,
//   baseVolume24h: 123,
//   quoteVolume24h: 12345
// }
