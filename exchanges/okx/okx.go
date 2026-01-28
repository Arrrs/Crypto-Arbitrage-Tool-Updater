package okx

import (
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
	instrumentsURL   = "https://www.okx.com/api/v5/market/tickers?instType=SPOT"
	MAX_DECIMAL_18_8 = 9999999999.99999999   // Максимальне значення для DECIMAL(18,8)
	MAX_DECIMAL_10_2 = 99999999.99           // Максимальне значення для DECIMAL(10,2)
	MAX_DECIMAL_20_2 = 999999999999999999.99 // Максимальне значення для DECIMAL(20,2)
)

type TickerResponse struct {
	Code string `json:"code"`
	Data []struct {
		InstID      string `json:"instId"`
		Last        string `json:"last"`
		BaseVolume  string `json:"vol24h"`
		QuoteVolume string `json:"volCcy24h"`
		Change24h   string `json:"change24h,omitempty"`
		Open24h     string `json:"open24h,omitempty"`
	} `json:"data"`
}

func fetchJSON(url string, target interface{}, wg *sync.WaitGroup, errChan chan<- error) {
	defer wg.Done()

	resp, err := http.Get(url)
	if err != nil {
		errChan <- fmt.Errorf("OKX error fetching %s: %w", url, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errChan <- fmt.Errorf("OKX non-OK status code %d from %s", resp.StatusCode, url)
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		errChan <- fmt.Errorf("OKX error reading response from %s: %w", url, err)
		return
	}

	if err := json.Unmarshal(body, target); err != nil {
		errChan <- fmt.Errorf("OKX error unmarshalling JSON from %s: %w", url, err)
	}
}

func parseFloat(s string, sym string) float64 {
	if s == "" {
		return 0
	}
	val, err := strconv.ParseFloat(s, 64)
	if err != nil {
		log.Printf("OKX Warning: failed to parse float from %s: %v. Symbol: %s", s, err, sym)
		return 0
	}
	return val
}

func sanitizeDecimal(value float64, maxValue float64, precision int) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}

	if value > maxValue {
		value = maxValue
	} else if value < -maxValue {
		value = -maxValue
	}

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

func calculatePercentChange(open, close float64) float64 {
	if open == 0 {
		return 0
	}
	return ((close - open) / open) * 100
}

func UpdateAllSpotPairs(db *sql.DB) bool {
	var wg sync.WaitGroup
	errChan := make(chan error, 1)

	var tickerData TickerResponse

	wg.Add(1)
	go fetchJSON(instrumentsURL, &tickerData, &wg, errChan)

	wg.Wait()
	close(errChan)

	for err := range errChan {
		if err != nil {
			log.Printf("Error: %v", err)
			return false
		}
	}

	var pairs []models.Pair
	for _, data := range tickerData.Data {
		symbolParts := strings.Split(data.InstID, "-")
		if len(symbolParts) != 2 {
			continue
		}

		baseAsset := symbolParts[0]
		quoteAsset := symbolParts[1]

		price := sanitizeDecimal(parseFloat(data.Last, data.InstID+"price"), MAX_DECIMAL_18_8, 8)
		baseVolume := sanitizeDecimal(parseFloat(data.BaseVolume, data.InstID+"baseVolume"), MAX_DECIMAL_20_2, 2)
		quoteVolume := sanitizeDecimal(parseFloat(data.QuoteVolume, data.InstID+"quoteVolume"), MAX_DECIMAL_20_2, 2)

		var priceChangePercent float64
		if data.Change24h != "" {
			priceChangePercent = sanitizeDecimal(parseFloat(data.Change24h, data.InstID+"priceChangePercent"), MAX_DECIMAL_10_2, 2)
		} else if data.Open24h != "" {
			openPrice := parseFloat(data.Open24h, data.InstID+"openPrice")
			priceChangePercent = sanitizeDecimal(calculatePercentChange(openPrice, price), MAX_DECIMAL_10_2, 2)
		} else {
			priceChangePercent = 0
		}

		if price <= 0 || math.IsNaN(price) || math.IsInf(price, 0) {
			continue
		}

		pair := models.Pair{
			PairKey:               fmt.Sprintf("%s_OKX_spot", strings.ReplaceAll(data.InstID, "-", "")),
			Symbol:                strings.ReplaceAll(data.InstID, "-", ""),
			Exchange:              "OKX",
			Market:                "spot",
			Price:                 price,
			BaseAsset:             baseAsset,
			QuoteAsset:            quoteAsset,
			DisplayName:           fmt.Sprintf("%s/%s", baseAsset, quoteAsset),
			PriceChangePercent24h: priceChangePercent,
			BaseVolume24h:         baseVolume,
			QuoteVolume24h:        quoteVolume,
			UpdatedAt:             time.Now(),
		}
		pairs = append(pairs, pair)
	}

	tx, err := db.Begin()
	if err != nil {
		log.Printf("OKX Failed to begin transaction: %v", err)
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
		log.Printf("OKX Failed to prepare statement: %v", err)
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
		log.Printf("OKX Failed to execute statement: %v", err)
		return false
	}

	if err := tx.Commit(); err != nil {
		log.Printf("OKX Failed to commit transaction: %v", err)
		return false
	}

	return true
}
