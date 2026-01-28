// kraken.go
package kraken

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"Updater/models"
)

const (
	symbolsURL = "https://api.kraken.com/0/public/AssetPairs"
	tickerURL  = "https://api.kraken.com/0/public/Ticker"
)

type SymbolsResponse struct {
	Result map[string]struct {
		Base  string `json:"base"`
		Quote string `json:"quote"`
	} `json:"result"`
}

type TickerResponse struct {
	Result map[string]struct {
		Last []string `json:"c"`
		Vol  []string `json:"v"`
	} `json:"result"`
}

func fetchJSON(url string, target interface{}) error {
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("Kraken error fetching %s: %w", url, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("Kraken error reading response: %w", err)
	}

	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("Kraken error unmarshalling JSON: %w", err)
	}
	return nil
}

func parseFloat(s string) float64 {
	val, err := strconv.ParseFloat(s, 64)
	if err != nil {
		log.Printf("Kraken Warning: failed to parse float from %s", s)
		return 0
	}
	return val
}

func UpdateAllSpotPairs(db *sql.DB) bool {
	var wg sync.WaitGroup
	var symbols SymbolsResponse
	var tickers TickerResponse
	var errChan = make(chan error, 2)

	wg.Add(2)
	go func() {
		errChan <- fetchJSON(symbolsURL, &symbols)
		wg.Done()
	}()
	go func() {
		errChan <- fetchJSON(tickerURL, &tickers)
		wg.Done()
	}()
	wg.Wait()
	close(errChan)

	for err := range errChan {
		if err != nil {
			log.Println("Kraken Error:", err)
			return false
		}
	}

	var pairs []models.Pair
	for symbol, info := range symbols.Result {
		if ticker, exists := tickers.Result[symbol]; exists {
			pair := models.Pair{
				PairKey:               fmt.Sprintf("%s_Kraken_spot", symbol),
				Symbol:                symbol,
				Exchange:              "Kraken",
				Market:                "spot",
				Price:                 parseFloat(ticker.Last[0]),
				BaseAsset:             info.Base,
				QuoteAsset:            info.Quote,
				DisplayName:           fmt.Sprintf("%s/%s", info.Base, info.Quote),
				PriceChangePercent24h: 0,
				BaseVolume24h:         parseFloat(ticker.Vol[1]),
				QuoteVolume24h:        0,
				UpdatedAt:             time.Now(),
			}
			pairs = append(pairs, pair)
		}
	}

	return savePairsToDB(db, pairs)
}

func savePairsToDB(db *sql.DB, pairs []models.Pair) bool {
	if len(pairs) == 0 {
		log.Println("Kraken No pairs to update")
		return false
	}
	tx, err := db.Begin()
	if err != nil {
		log.Println("Kraken Failed to begin transaction:", err)
		return false
	}

	query := "INSERT INTO pairs (pairkey, symbol, exchange, market, price, baseasset, quoteasset, displayname, pricechangepercent24h, basevolume24h, quotevolume24h, updatedat, createdat) VALUES "
	placeholders := []string{}
	args := []interface{}{}

	for i, pair := range pairs {
		placeholders = append(placeholders, fmt.Sprintf("($%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d)", i*13+1, i*13+2, i*13+3, i*13+4, i*13+5, i*13+6, i*13+7, i*13+8, i*13+9, i*13+10, i*13+11, i*13+12, i*13+13))
		args = append(args, pair.PairKey, pair.Symbol, pair.Exchange, pair.Market, pair.Price, pair.BaseAsset, pair.QuoteAsset, pair.DisplayName, pair.PriceChangePercent24h, pair.BaseVolume24h, pair.QuoteVolume24h, pair.UpdatedAt, time.Now())
	}

	query += strings.Join(placeholders, ", ") + " ON CONFLICT (pairkey) DO UPDATE SET price = EXCLUDED.price, basevolume24h = EXCLUDED.basevolume24h, quotevolume24h = EXCLUDED.quotevolume24h, updatedat = EXCLUDED.updatedat"
	_, err = tx.Exec(query, args...)
	if err != nil {
		tx.Rollback()
		log.Println("Kraken Failed to execute statement:", err)
		return false
	}

	if err := tx.Commit(); err != nil {
		log.Println("Kraken Failed to commit transaction:", err)
		return false
	}

	return true
}
