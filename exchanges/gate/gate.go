package gate

import (
	"crypto/tls"
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

	"Updater/models"
)

const (
	baseURL          = "https://api.gateio.ws/api/v4"
	currencyPairsURL = baseURL + "/spot/currency_pairs"
	tickerPricesURL  = baseURL + "/spot/tickers"
)

type CurrencyPairsResponse struct {
	ID          string `json:"id"`
	Base        string `json:"base"`
	Quote       string `json:"quote"`
	TradeStatus string `json:"trade_status"`
}

type TickerResponse struct {
	CurrencyPair         string `json:"currency_pair"`
	LastPrice            string `json:"last"`
	PriceChangePercent24 string `json:"change_percentage"`
	BaseVolume24h        string `json:"base_volume"`
	QuoteVolume24h       string `json:"quote_volume"`
}

func fetchJSON(url string, target interface{}, wg *sync.WaitGroup, errChan chan error) {
	defer wg.Done()
	// Create a custom HTTP client with TLS certificate verification disabled
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	resp, err := client.Get(url)
	if err != nil {
		errChan <- fmt.Errorf("Gate.io error fetching %s: %w", url, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("Gate.io non-OK status code %d: %s", resp.StatusCode, string(body))
		errChan <- fmt.Errorf("Gate.io non-OK status code %d from %s", resp.StatusCode, url)
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		errChan <- fmt.Errorf("Gate.io error reading response: %w", err)
		return
	}

	// Log the response body if unmarshalling fails
	if err := json.Unmarshal(body, target); err != nil {
		log.Printf("Gate.io response body: %s", string(body))
		errChan <- fmt.Errorf("Gate.io error unmarshalling JSON: %w", err)
		return
	}
}

func parseFloat(s string) float64 {
	val, err := strconv.ParseFloat(s, 64)
	if err != nil {
		log.Printf("Gate.io Warning: failed to parse float from %s", s)
		return 0
	}
	return val
}

func roundToPrecision(value float64, precision int) float64 {
	format := "%." + strconv.Itoa(precision) + "f"
	strValue := fmt.Sprintf(format, value)
	roundedValue, _ := strconv.ParseFloat(strValue, 64)
	return roundedValue
}

func validateFloat64(value float64, precision int, scale int) float64 {
	maxValue := math.Pow10(precision - scale)
	if value > maxValue {
		log.Printf("Gate.io Warning: value %f exceeds max %f, setting to %f", value, maxValue, maxValue)
		return maxValue
	}
	if value < -maxValue {
		log.Printf("Gate.io Warning: value %f below min %f, setting to %f", value, -maxValue, -maxValue)
		return -maxValue
	}
	return roundToPrecision(value, scale)
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

	var currencyPairs []CurrencyPairsResponse
	var tickers []TickerResponse

	wg.Add(2)
	go fetchJSON(currencyPairsURL, &currencyPairs, &wg, errChan)
	go fetchJSON(tickerPricesURL, &tickers, &wg, errChan)

	wg.Wait()
	close(errChan)

	for err := range errChan {
		if err != nil {
			log.Printf("Error: %v", err)
			return false
		}
	}

	priceMap := make(map[string]TickerResponse)
	for _, t := range tickers {
		priceMap[t.CurrencyPair] = t
	}

	var pairs []models.Pair
	for _, sym := range currencyPairs {
		if sym.TradeStatus != "tradable" {
			continue
		}

		ticker := priceMap[sym.ID]

		pair := models.Pair{
			PairKey:               fmt.Sprintf("%s_Gate_spot", strings.ReplaceAll(sym.ID, "_", "")),
			Symbol:                strings.ReplaceAll(sym.ID, "_", ""),
			Exchange:              "Gate",
			Market:                "spot",
			Price:                 validateFloat64(parseFloat(ticker.LastPrice), 18, 8), // 8 decimal places
			BaseAsset:             sym.Base,
			QuoteAsset:            sym.Quote,
			DisplayName:           fmt.Sprintf("%s/%s", sym.Base, sym.Quote),
			PriceChangePercent24h: validateFloat64(parseFloat(ticker.PriceChangePercent24), 10, 2), // 2 decimal places
			BaseVolume24h:         validateFloat64(parseFloat(ticker.BaseVolume24h), 20, 2),        // 2 decimal places
			QuoteVolume24h:        validateFloat64(parseFloat(ticker.QuoteVolume24h), 20, 2),       // 2 decimal places
			UpdatedAt:             time.Now(),
		}
		pairs = append(pairs, pair)
	}

	tx, err := db.Begin()
	if err != nil {
		log.Printf("Gate.io Failed to begin transaction: %v", err)
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
		log.Printf("Gate.io Failed to prepare statement: %v", err)
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
		log.Printf("Gate.io Failed to execute statement: %v", err)
		return false
	}

	if err := tx.Commit(); err != nil {
		log.Printf("Gate.io Failed to commit transaction: %v", err)
		return false
	}

	return true
}
