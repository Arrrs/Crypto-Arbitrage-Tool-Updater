package mexc

import (
	"Updater/models"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	symbolsURL       = "https://api.mexc.com/api/v3/exchangeInfo"
	tickerURL        = "https://api.mexc.com/api/v3/ticker/24hr"
	futuresTickerURL = "https://contract.mexc.com/api/v1/contract/ticker"
)

type SymbolResponse struct {
	Symbols []struct {
		Symbol     string `json:"symbol"`
		BaseAsset  string `json:"baseAsset"`
		QuoteAsset string `json:"quoteAsset"`
		Status     bool   `json:"isSpotTradingAllowed"`
	} `json:"symbols"`
}

type TickerResponse struct {
	Symbol                string `json:"symbol"`
	LastPrice             string `json:"lastPrice"`
	PriceChangePercent24h string `json:"priceChangePercent"`
	BaseVolume24h         string `json:"volume"`
	QuoteVolume24h        string `json:"quoteVolume"`
}

type FuturesTickerResponse struct {
	Data []struct {
		Symbol      string  `json:"symbol"`
		IndexPrice  float64 `json:"indexPrice"`
		FairPrice   float64 `json:"fairPrice"`
		FundingRate float64 `json:"fundingRate"`
		Volume24    float64 `json:"volume24"`
	} `json:"data"`
}

func fetchJSON(url string, target interface{}, wg *sync.WaitGroup, errChan chan<- error) {
	defer wg.Done()

	resp, err := http.Get(url)
	if err != nil {
		errChan <- fmt.Errorf("MEXC error fetching %s: %w", url, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errChan <- fmt.Errorf("MEXC non-OK status code %d from %s", resp.StatusCode, url)
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		errChan <- fmt.Errorf("MEXC error reading response from %s: %w", url, err)
		return
	}

	if err := json.Unmarshal(body, target); err != nil {
		errChan <- fmt.Errorf("MEXC error unmarshalling JSON from %s: %w", url, err)
	}
}

func parseFloat(s string) float64 {
	val, err := strconv.ParseFloat(s, 64)
	if err != nil {
		log.Printf("MEXC Warning: failed to parse float from %s", s)
		return 0
	}
	return val
}

func formatFloat(val float64, precision int) float64 {
	format := "%." + strconv.Itoa(precision) + "f"
	strVal := fmt.Sprintf(format, val)
	formattedVal, _ := strconv.ParseFloat(strVal, 64)
	return formattedVal
}

func limitPrecision(val float64, maxDigits int) float64 {
	format := "%." + strconv.Itoa(maxDigits) + "f"
	strVal := fmt.Sprintf(format, val)
	limitedVal, _ := strconv.ParseFloat(strVal, 64)
	return limitedVal
}

func sanitizeDecimal(value float64, maxValue float64, precision int) float64 {
	// Check for NaN and Inf
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}

	// Clamp value
	if value > maxValue {
		value = maxValue
	} else if value < -maxValue {
		value = -maxValue
	}

	// Format with required precision
	format := "%." + strconv.Itoa(precision) + "f"
	strVal := fmt.Sprintf(format, value)
	formattedVal, _ := strconv.ParseFloat(strVal, 64)
	return formattedVal
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
	errChan := make(chan error, 2)

	var symbols SymbolResponse
	var tickerData []TickerResponse

	wg.Add(2)
	go fetchJSON(symbolsURL, &symbols, &wg, errChan)
	go fetchJSON(tickerURL, &tickerData, &wg, errChan)

	wg.Wait()
	close(errChan)

	for err := range errChan {
		if err != nil {
			log.Printf("MEXC Error: %v", err)
			return false
		}
	}

	symbolMap := make(map[string]struct {
		Base  string
		Quote string
	})
	for _, s := range symbols.Symbols {
		if s.Status {
			symbolMap[s.Symbol] = struct {
				Base  string
				Quote string
			}{Base: s.BaseAsset, Quote: s.QuoteAsset}
		}
	}

	var pairs []models.Pair
	for _, t := range tickerData {
		symbolInfo, exists := symbolMap[t.Symbol]
		if !exists {
			continue
		}

		price := sanitizeDecimal(parseFloat(t.LastPrice), 9999999999.99999999, 8)
		priceChangePercent24h := sanitizeDecimal(parseFloat(t.PriceChangePercent24h), 99999999.99, 2)
		baseVolume24h := sanitizeDecimal(parseFloat(t.BaseVolume24h), 999999999999999999.99, 2)
		quoteVolume24h := sanitizeDecimal(parseFloat(t.QuoteVolume24h), 999999999999999999.99, 2)

		// Перевірка на неприпустимі значення для полів price, baseVolume і quoteVolume
		if price <= 0 || math.IsNaN(price) || math.IsInf(price, 0) {
			// log.Printf("Skipping pair %s due to invalid price", t.Symbol)
			continue
		}

		pair := models.Pair{
			PairKey:               fmt.Sprintf("%s_MEXC_spot", t.Symbol),
			Symbol:                t.Symbol,
			Exchange:              "MEXC",
			Market:                "spot",
			Price:                 price,
			BaseAsset:             symbolInfo.Base,
			QuoteAsset:            symbolInfo.Quote,
			DisplayName:           fmt.Sprintf("%s/%s", symbolInfo.Base, symbolInfo.Quote),
			PriceChangePercent24h: priceChangePercent24h,
			BaseVolume24h:         baseVolume24h,
			QuoteVolume24h:        quoteVolume24h,
			UpdatedAt:             time.Now(),
		}
		pairs = append(pairs, pair)
	}

	if len(pairs) == 0 {
		log.Printf("MEXC No pairs to update")
		return false
	}

	tx, err := db.Begin()
	if err != nil {
		log.Printf("MEXC Failed to begin transaction: %v", err)
		return false
	}

	placeholderStr := generateNumberedPlaceholders(len(pairs), 12)
	query := `
    INSERT INTO pairs (pairkey, symbol, exchange, market, price, baseasset, quoteasset, displayname, pricechangepercent24h, basevolume24h, quotevolume24h, updatedat)
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
		log.Printf("MEXC Failed to prepare statement: %v", err)
		return false
	}
	defer stmt.Close()

	args := make([]interface{}, 0, len(pairs)*12)
	for _, pair := range pairs {
		args = append(args, pair.PairKey, pair.Symbol, pair.Exchange, pair.Market, pair.Price, pair.BaseAsset, pair.QuoteAsset,
			pair.DisplayName, pair.PriceChangePercent24h, pair.BaseVolume24h, pair.QuoteVolume24h, pair.UpdatedAt)
	}

	_, err = stmt.Exec(args...)
	if err != nil {
		tx.Rollback()
		log.Printf("MEXC Failed to execute statement: %v", err)
		return false
	}

	if err := tx.Commit(); err != nil {
		log.Printf("MEXC Failed to commit transaction: %v", err)
		return false
	}

	return true
}

func UpdateAllFuturesPairs(db *sql.DB) bool {
	var wg sync.WaitGroup
	errChan := make(chan error, 1)

	var futuresData FuturesTickerResponse

	wg.Add(1)
	go fetchJSON(futuresTickerURL, &futuresData, &wg, errChan)

	wg.Wait()
	close(errChan)

	for err := range errChan {
		if err != nil {
			log.Printf("MEXC Error: %v", err)
			return false
		}
	}

	var pairs []models.PairFutures
	for _, data := range futuresData.Data {
		// Split symbol to get baseAsset and quoteAsset
		symbolParts := strings.Split(data.Symbol, "_")
		if len(symbolParts) != 2 {
			log.Printf("MEXC Warning: Invalid symbol format %s", data.Symbol)
			continue
		}
		baseAsset := symbolParts[0]
		quoteAsset := symbolParts[1]

		// Calculate quoteVolume24h
		quoteVolume24h := data.Volume24 * data.FairPrice

		// Create PairFutures object
		pair := models.PairFutures{
			PairKey:     fmt.Sprintf("%s_MEXC_futures", strings.ReplaceAll(data.Symbol, "_", "")),
			Symbol:      strings.ReplaceAll(data.Symbol, "_", ""),
			Exchange:    "MEXC",
			Market:      "futures",
			MarkPrice:   formatFloat(data.FairPrice, 8),
			IndexPrice:  formatFloat(data.IndexPrice, 8),
			BaseAsset:   baseAsset,
			QuoteAsset:  quoteAsset,
			DisplayName: fmt.Sprintf("%s/%s", baseAsset, quoteAsset),
			// FundingRatePercent:    formatFloat(data.FundingRate*100, 6), // Convert to percentage
			FundingRatePercent:    formatFloat(data.FundingRate, 6), // Convert to percentage
			NextFundingTimestamp:  0,                                // Not available in the endpoint
			PriceChangePercent24h: 0,                                // Not provided in the endpoint
			BaseVolume24h:         formatFloat(data.Volume24, 2),
			QuoteVolume24h:        formatFloat(quoteVolume24h, 2),
			UpdatedAt:             time.Now(),
		}
		pairs = append(pairs, pair)
	}

	if len(pairs) == 0 {
		log.Printf("MEXC No futures pairs to update")
		return false
	}

	tx, err := db.Begin()
	if err != nil {
		log.Printf("MEXC Failed to begin transaction: %v", err)
		return false
	}

	placeholderStr := generateNumberedPlaceholders(len(pairs), 15)
	query := `
    INSERT INTO pairsfutures (pairkey, symbol, exchange, market, markprice, indexprice, baseasset, quoteasset, displayname, fundingRatePercent, nextfundingtimestamp, pricechangepercent24h, basevolume24h, quotevolume24h, updatedat)
    VALUES ` + placeholderStr + `
    ON CONFLICT (pairkey) DO UPDATE SET
        markprice = EXCLUDED.markprice,
        indexprice = EXCLUDED.indexprice,
        fundingRatePercent = EXCLUDED.fundingRatePercent,
        nextfundingtimestamp = EXCLUDED.nextfundingtimestamp,
        basevolume24h = EXCLUDED.basevolume24h,
        quotevolume24h = EXCLUDED.quotevolume24h,
        updatedat = EXCLUDED.updatedat
    `
	stmt, err := tx.Prepare(query)
	if err != nil {
		log.Printf("MEXC Failed to prepare statement: %v", err)
		return false
	}
	defer stmt.Close()

	args := make([]interface{}, 0, len(pairs)*15)
	for _, pair := range pairs {
		args = append(
			args,
			pair.PairKey,
			pair.Symbol,
			pair.Exchange,
			pair.Market,
			pair.MarkPrice,
			pair.IndexPrice,
			pair.BaseAsset,
			pair.QuoteAsset,
			pair.DisplayName,
			pair.FundingRatePercent,
			pair.NextFundingTimestamp,
			pair.PriceChangePercent24h,
			pair.BaseVolume24h,
			pair.QuoteVolume24h,
			pair.UpdatedAt,
		)
	}

	_, err = stmt.Exec(args...)
	if err != nil {
		tx.Rollback()
		log.Printf("MEXC Failed to execute statement: %v", err)
		return false
	}

	if err := tx.Commit(); err != nil {
		log.Printf("MEXC Failed to commit transaction: %v", err)
		return false
	}

	return true
}
