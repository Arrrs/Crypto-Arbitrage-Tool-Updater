package bitget

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
	marketListURL  = "https://api.bitget.com/api/v2/spot/public/symbols"
	tickerPriceURL = "https://api.bitget.com/api/v2/spot/market/tickers"
	networkInfoURL = "https://api.bitget.com/api/v2/spot/public/coins"
)

type MarketListResponse struct {
	Data []struct {
		Symbol      string `json:"symbol"`
		BaseCoin    string `json:"baseCoin"`
		QuoteCoin   string `json:"quoteCoin"`
		Status      string `json:"status"`
		MinTradeAmt string `json:"minTradeAmount"`
	} `json:"data"`
}

type TickerPriceResponse struct {
	Data []struct {
		Symbol           string `json:"symbol"`
		Price            string `json:"lastPr"`
		ChangePercent24h string `json:"change24h"`
		BaseVolume24h    string `json:"baseVolume"`
		QuoteVolume24h   string `json:"quoteVolume"`
	} `json:"data"`
}

func fetchJSON(url string, target interface{}, wg *sync.WaitGroup, errChan chan<- error) {
	defer wg.Done()

	resp, err := http.Get(url)
	if err != nil {
		errChan <- fmt.Errorf("bitget error fetching %s: %w", url, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errChan <- fmt.Errorf("bitget non-OK status code %d from %s", resp.StatusCode, url)
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		errChan <- fmt.Errorf("bitget error reading response from %s: %w", url, err)
		return
	}

	if err := json.Unmarshal(body, target); err != nil {
		errChan <- fmt.Errorf("bitget error unmarshalling JSON from %s: %w", url, err)
	}
}

func parseFloat(s string, d string) float64 {
	val, err := strconv.ParseFloat(s, 64)
	if err != nil {
		log.Printf("Bitget Warning: failed to parse float from %s, field: %v", s, d)
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

	var marketList MarketListResponse
	var tickerData TickerPriceResponse

	wg.Add(2)
	go fetchJSON(marketListURL, &marketList, &wg, errChan)
	go fetchJSON(tickerPriceURL, &tickerData, &wg, errChan)

	wg.Wait()
	close(errChan)

	for err := range errChan {
		if err != nil {
			log.Printf("Error: %v", err)
			return false
		}
	}

	priceMap := make(map[string]struct {
		Symbol           string
		Price            string
		ChangePercent24h string
		BaseVolume24h    string
		QuoteVolume24h   string
	})
	for _, t := range tickerData.Data {
		priceMap[t.Symbol] = struct {
			Symbol           string
			Price            string
			ChangePercent24h string
			BaseVolume24h    string
			QuoteVolume24h   string
		}{
			Symbol:           t.Symbol,
			Price:            t.Price,
			ChangePercent24h: t.ChangePercent24h,
			BaseVolume24h:    t.BaseVolume24h,
			QuoteVolume24h:   t.QuoteVolume24h,
		}
	}

	var pairs []models.Pair
	for _, sym := range marketList.Data {
		if sym.Status != "online" {
			continue
		}

		ticker, exists := priceMap[sym.Symbol]
		if !exists {
			continue
		}

		pair := models.Pair{
			PairKey:               fmt.Sprintf("%s_Bitget_spot", sym.Symbol),
			Symbol:                sym.Symbol,
			Exchange:              "Bitget",
			Market:                "spot",
			Price:                 formatFloat(parseFloat(ticker.Price, "Price"), 8),
			BaseAsset:             sym.BaseCoin,
			QuoteAsset:            sym.QuoteCoin,
			DisplayName:           fmt.Sprintf("%s/%s", sym.BaseCoin, sym.QuoteCoin),
			PriceChangePercent24h: formatFloat(parseFloat(ticker.ChangePercent24h, "PriceChangePercent24h"), 2),
			BaseVolume24h:         formatFloat(parseFloat(ticker.BaseVolume24h, "BaseVolume24h"), 2),
			QuoteVolume24h:        formatFloat(parseFloat(ticker.QuoteVolume24h, "QuoteVolume24h"), 2),
			UpdatedAt:             time.Now(),
		}
		pairs = append(pairs, pair)
	}

	tx, err := db.Begin()
	if err != nil {
		log.Printf("Bitget Failed to begin transaction: %v", err)
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
		log.Printf("Bitget Failed to prepare statement: %v", err)
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
		log.Printf("Bitget Failed to execute statement: %v", err)
		return false
	}

	if err := tx.Commit(); err != nil {
		log.Printf("Bitget Failed to commit transaction: %v", err)
		return false
	}

	return true
}

func UpdateAllNetworks(db *sql.DB) bool {
	type Chain struct {
		Chain             string `json:"chain"`
		NeedTag           string `json:"needTag"`
		Withdrawable      string `json:"withdrawable"`
		Rechargeable      string `json:"rechargeable"`
		DepositConfirm    string `json:"depositConfirm"`
		WithdrawConfirm   string `json:"withdrawConfirm"`
		MinDepositAmount  string `json:"minDepositAmount"`
		MinWithdrawAmount string `json:"minWithdrawAmount"`
		BrowserUrl        string `json:"browserUrl"`
	}

	type CoinData struct {
		CoinId   string  `json:"coinId"`
		Coin     string  `json:"coin"`
		Transfer string  `json:"transfer"`
		Chains   []Chain `json:"chains"`
	}

	type NetworkInfoResponse struct {
		Code string     `json:"code"`
		Msg  string     `json:"msg"`
		Data []CoinData `json:"data"`
	}

	var networkInfo NetworkInfoResponse

	// Fetch network data from Bitget API
	resp, err := http.Get(networkInfoURL)
	if err != nil {
		log.Printf("Bitget error fetching network info: %v", err)
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("Bitget non-OK status code %d from %s", resp.StatusCode, networkInfoURL)
		return false
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Bitget error reading response: %v", err)
		return false
	}

	if err := json.Unmarshal(body, &networkInfo); err != nil {
		log.Printf("Bitget error unmarshalling JSON: %v", err)
		return false
	}

	tx, err := db.Begin()
	if err != nil {
		log.Printf("Bitget Failed to begin transaction: %v", err)
		return false
	}

	// Prepare SQL query with ON CONFLICT
	query := `
    INSERT INTO nets (coinKey, coin, exchange, network, networkName, depositEnable, withdrawEnable, updatedAt)
    VALUES %s
    ON CONFLICT (coinKey) DO UPDATE SET
        networkName = EXCLUDED.networkName,
        depositEnable = EXCLUDED.depositEnable,
        withdrawEnable = EXCLUDED.withdrawEnable,
        updatedAt = EXCLUDED.updatedAt
    `

	var values []string
	var args []interface{}
	counter := 1

	for _, coin := range networkInfo.Data {
		for _, chain := range coin.Chains {
			coinKey := fmt.Sprintf("%s_Bitget_%s", coin.Coin, chain.Chain)
			values = append(values, fmt.Sprintf("($%d, $%d, 'Bitget', $%d, $%d, $%d, $%d, $%d)", counter, counter+1, counter+2, counter+3, counter+4, counter+5, counter+6))
			args = append(args, coinKey, coin.Coin, chain.Chain, chain.Chain, chain.Rechargeable == "true", chain.Withdrawable == "true", time.Now().UTC())
			counter += 7
		}
	}

	if len(values) == 0 {
		log.Println("Bitget: No network data to update")
		tx.Rollback()
		return true
	}

	fullQuery := fmt.Sprintf(query, strings.Join(values, ", "))
	_, err = tx.Exec(fullQuery, args...)
	if err != nil {
		tx.Rollback()
		log.Printf("Bitget Failed to execute statement: %v", err)
		return false
	}

	if err := tx.Commit(); err != nil {
		log.Printf("Bitget Failed to commit transaction: %v", err)
		return false
	}

	// log.Println("Bitget: Successfully updated network availability")
	return true
}
