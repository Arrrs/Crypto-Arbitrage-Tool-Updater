package kucoin

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
	symbolsURL    = "https://api.kucoin.com/api/v1/symbols"
	tickerURL     = "https://api.kucoin.com/api/v1/market/allTickers"
	currenciesURL = "https://api.kucoin.com/api/v3/currencies"
)

type SymbolResponse struct {
	Data []struct {
		Symbol        string `json:"symbol"`
		BaseAsset     string `json:"baseCurrency"`
		QuoteAsset    string `json:"quoteCurrency"`
		EnableTrading bool   `json:"enableTrading"`
	} `json:"data"`
}

type TickerResponse struct {
	Data struct {
		Tickers []struct {
			Symbol    string `json:"symbol"`
			Last      string `json:"last"`
			Change24h string `json:"changeRate"`
			BaseVol   string `json:"volValue"`
		} `json:"ticker"`
	} `json:"data"`
}

func fetchJSON(url string, target interface{}, wg *sync.WaitGroup, errChan chan<- error) {
	defer wg.Done()

	resp, err := http.Get(url)
	if err != nil {
		errChan <- fmt.Errorf("KuCoin error fetching %s: %w", url, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errChan <- fmt.Errorf("KuCoin non-OK status code %d from %s", resp.StatusCode, url)
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		errChan <- fmt.Errorf("KuCoin error reading response from %s: %w", url, err)
		return
	}

	if err := json.Unmarshal(body, target); err != nil {
		errChan <- fmt.Errorf("KuCoin error unmarshalling JSON from %s: %w", url, err)
	}
}

func parseFloat(s string, d string) float64 {
	val, err := strconv.ParseFloat(s, 64)
	if err != nil {
		log.Printf("KuCoin Warning: failed to parse float from %s, field: %v", s, d)
		return 0
	}
	return val
}

func limitFloat(value float64, min float64, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func generateNumberedPlaceholders(rows int, fieldCount int) string {
	placeholders := make([]string, rows)
	counter := 1
	for i := 0; i < rows; i++ {
		inner := make([]string, fieldCount)
		for j := 0; j < fieldCount; j++ {
			inner[j] = "$" + strconv.Itoa(counter)
			counter++
		}
		placeholders[i] = "(" + strings.Join(inner, ", ") + ")"
	}
	return strings.Join(placeholders, ", ")
}

func UpdateAllSpotPairs(db *sql.DB) bool {
	var wg sync.WaitGroup
	errChan := make(chan error, 1)

	var symbols SymbolResponse
	var tickerData TickerResponse

	wg.Add(2)
	go fetchJSON(symbolsURL, &symbols, &wg, errChan)
	go fetchJSON(tickerURL, &tickerData, &wg, errChan)

	wg.Wait()
	close(errChan)

	for err := range errChan {
		if err != nil {
			log.Printf("KuCoin Error: %v", err)
			return false
		}
	}

	symbolMap := make(map[string]struct {
		Base  string
		Quote string
	})
	for _, s := range symbols.Data {
		if s.EnableTrading {
			symbolMap[s.Symbol] = struct {
				Base  string
				Quote string
			}{Base: s.BaseAsset, Quote: s.QuoteAsset}
		}
	}

	var pairs []models.Pair
	for _, t := range tickerData.Data.Tickers {
		symbolInfo, exists := symbolMap[t.Symbol]
		if !exists {
			continue
		}

		price := parseFloat(t.Last, t.Symbol)
		priceChangePercent24h := parseFloat(t.Change24h, "Price%Change") * 100
		baseVolume24h := parseFloat(t.BaseVol, "BaseVolume24h")

		// Limit the values to avoid numeric field overflow
		price = limitFloat(price, -1e10, 1e10)
		priceChangePercent24h = limitFloat(priceChangePercent24h, -1e10, 1e10)
		baseVolume24h = limitFloat(baseVolume24h, -1e10, 1e10)

		pair := models.Pair{
			PairKey:               fmt.Sprintf("%s_KuCoin_spot", strings.ReplaceAll(t.Symbol, "-", "")),
			Symbol:                strings.ReplaceAll(t.Symbol, "-", ""),
			Exchange:              "KuCoin",
			Market:                "spot",
			Price:                 price,
			BaseAsset:             symbolInfo.Base,
			QuoteAsset:            symbolInfo.Quote,
			DisplayName:           fmt.Sprintf("%s/%s", symbolInfo.Base, symbolInfo.Quote),
			PriceChangePercent24h: priceChangePercent24h,
			BaseVolume24h:         baseVolume24h,
			QuoteVolume24h:        0,
			UpdatedAt:             time.Now(),
		}
		pairs = append(pairs, pair)
	}

	tx, err := db.Begin()
	if err != nil {
		log.Printf("KuCoin Failed to begin transaction: %v", err)
		return false
	}

	placeholderStr := generateNumberedPlaceholders(len(pairs), 13)
	query := `
	INSERT INTO pairs (pairkey, symbol, exchange, market, price, baseasset, quoteasset, displayname, pricechangepercent24h, basevolume24h, quotevolume24h, updatedat, createdat)
	VALUES ` + placeholderStr + `
	ON CONFLICT (pairkey) DO UPDATE SET
		price = EXCLUDED.price,
		pricechangepercent24h = EXCLUDED.pricechangepercent24h,
		basevolume24h = EXCLUDED.basevolume24h,
		quotevolume24h = EXCLUDED.quotevolume24h,
		updatedat = EXCLUDED.updatedat
	`
	stmt, err := tx.Prepare(query)
	if err != nil {
		log.Printf("KuCoin Failed to prepare statement: %v", err)
		return false
	}
	defer stmt.Close()

	args := make([]interface{}, 0, len(pairs)*13)
	for _, pair := range pairs {
		args = append(args, pair.PairKey, pair.Symbol, pair.Exchange, pair.Market, pair.Price, pair.BaseAsset, pair.QuoteAsset,
			pair.DisplayName, pair.PriceChangePercent24h, pair.BaseVolume24h, pair.QuoteVolume24h, pair.UpdatedAt, time.Now())
	}

	_, err = stmt.Exec(args...)
	if err != nil {
		tx.Rollback()
		log.Printf("KuCoin Failed to execute statement: %v", err)
		return false
	}

	if err := tx.Commit(); err != nil {
		log.Printf("KuCoin Failed to commit transaction: %v", err)
		return false
	}

	return true
}
