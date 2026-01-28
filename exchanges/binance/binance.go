package binance

import (
	"crypto/hmac"
	"crypto/sha256"
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
	exchangeInfoURL        = "https://api.binance.com/api/v3/exchangeInfo?permissions=SPOT&symbolStatus=TRADING"
	tickerPriceURL         = "https://api.binance.com/api/v3/ticker/price"
	ticker24hrURL          = "https://api.binance.com/api/v3/ticker/24hr"
	assetDetailURL         = "https://api.binance.com/sapi/v1/capital/config/getall"
	serverTimeURL          = "https://api.binance.com/api/v3/time"
	exchangeInfoFuturesURL = "https://fapi.binance.com/fapi/v1/exchangeInfo"
	ticker24hrFuturesURL   = "https://fapi.binance.com/fapi/v1/ticker/24hr"
	futuresDataURL         = "https://fapi.binance.com/fapi/v1/premiumIndex"
)

type AssetDetail struct {
	Coin        string `json:"coin"`
	NetworkList []struct {
		Network        string `json:"network"`
		Name           string `json:"name"`
		DepositEnable  bool   `json:"depositEnable"`
		WithdrawEnable bool   `json:"withdrawEnable"`
	} `json:"networkList"`
}

type ExchangeInfoResponse struct {
	Symbols []struct {
		Symbol               string `json:"symbol"`
		BaseAsset            string `json:"baseAsset"`
		QuoteAsset           string `json:"quoteAsset"`
		IsSpotTradingAllowed bool   `json:"isSpotTradingAllowed"`
	} `json:"symbols"`
}

type TickerPriceResponse struct {
	Symbol string `json:"symbol"`
	Price  string `json:"price"`
}

type Ticker24hrResponse struct {
	Symbol                string `json:"symbol"`
	PriceChangePercent24h string `json:"priceChangePercent"`
	BaseVolume24h         string `json:"volume"`
	QuoteVolume24h        string `json:"quoteVolume"`
}

type FuturesExchangeInfoResponse struct {
	Symbols []struct {
		Symbol                string `json:"symbol"`
		BaseAsset             string `json:"baseAsset"`
		QuoteAsset            string `json:"quoteAsset"`
		ContractType          string `json:"contractType"`
		PricePrecision        int    `json:"pricePrecision"`
		QuantityPrecision     int    `json:"quantityPrecision"`
		MaintMarginPercent    string `json:"maintMarginPercent"`
		RequiredMarginPercent string `json:"requiredMarginPercent"`
	} `json:"symbols"`
}

func fetchJSON(url string, target interface{}, wg *sync.WaitGroup, errChan chan<- error) {
	defer wg.Done()

	resp, err := http.Get(url)
	if err != nil {
		errChan <- fmt.Errorf("Binance error fetching %s: %w", url, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errChan <- fmt.Errorf("Binance non-OK status code %d from %s", resp.StatusCode, url)
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		errChan <- fmt.Errorf("Binance error reading response from %s: %w", url, err)
		return
	}

	if err := json.Unmarshal(body, target); err != nil {
		errChan <- fmt.Errorf("Binance error unmarshalling JSON from %s: %w", url, err)
	}
}

// parseFloat - хелпер для конвертації рядка в float64
func parseFloat(s string, d string) float64 {
	val, err := strconv.ParseFloat(s, 64)
	if err != nil {
		log.Printf("Binance Warning: failed to parse float from %s, description: %s", s, d)
		return 0
	}
	return val
}

// formatFloat - хелпер для форматування float64 з заданою кількістю десяткових знаків
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
	errChan := make(chan error, 3)

	// Змінні для збереження відповідей
	var exchangeInfo ExchangeInfoResponse
	var tickerPrices []TickerPriceResponse
	var ticker24hrs []Ticker24hrResponse

	// Запускаємо три паралельні запити
	wg.Add(3)
	go fetchJSON(exchangeInfoURL, &exchangeInfo, &wg, errChan)
	go fetchJSON(tickerPriceURL, &tickerPrices, &wg, errChan)
	go fetchJSON(ticker24hrURL, &ticker24hrs, &wg, errChan)

	// Чекаємо завершення всіх запитів
	wg.Wait()
	close(errChan)

	// Перевіряємо наявність помилок
	for err := range errChan {
		if err != nil {
			log.Printf("Error: %v", err)
			return false
		}
	}

	// Створюємо мапи для швидкого доступу до даних
	priceMap := make(map[string]float64)
	for _, p := range tickerPrices {
		priceMap[p.Symbol] = formatFloat(parseFloat(p.Price, "tickerPrices loop"), 8)
	}

	ticker24hrMap := make(map[string]Ticker24hrResponse)
	for _, t := range ticker24hrs {
		ticker24hrMap[t.Symbol] = t
	}

	// Формуємо фінальний масив `Pair`
	var pairs []models.Pair
	for _, sym := range exchangeInfo.Symbols {
		if !sym.IsSpotTradingAllowed {
			continue
		}

		price := priceMap[sym.Symbol]
		ticker24hr := ticker24hrMap[sym.Symbol]

		pair := models.Pair{
			PairKey:               fmt.Sprintf("%s_Binance_spot", sym.Symbol),
			Symbol:                sym.Symbol,
			Exchange:              "Binance",
			Market:                "spot",
			Price:                 formatFloat(price, 8),
			BaseAsset:             sym.BaseAsset,
			QuoteAsset:            sym.QuoteAsset,
			DisplayName:           fmt.Sprintf("%s/%s", sym.BaseAsset, sym.QuoteAsset),
			PriceChangePercent24h: formatFloat(parseFloat(ticker24hr.PriceChangePercent24h, "ticker24hr.PriceChangePercent24h"), 2),
			BaseVolume24h:         formatFloat(parseFloat(ticker24hr.BaseVolume24h, "ticker24hr.BaseVolume24h"), 2),
			QuoteVolume24h:        formatFloat(parseFloat(ticker24hr.QuoteVolume24h, "ticker24hr.QuoteVolume24h"), 2),
			UpdatedAt:             time.Now(),
		}
		pairs = append(pairs, pair)
	}

	tx, err := db.Begin()
	if err != nil {
		log.Printf("Binance Failed to begin transaction: %v", err)
		return false
	}

	// Using 12 columns per record
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
		log.Printf("Binance Failed to prepare statement: %v", err)
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
		log.Printf("Binance Failed to execute statement: %v", err)
		return false
	}

	if err := tx.Commit(); err != nil {
		log.Printf("Binance Failed to commit transaction: %v", err)
		return false
	}

	return true
}

func UpdateAllNetworks(db *sql.DB, apiKey, secretKey string) bool {
	if apiKey == "" || secretKey == "" {
		log.Println("Binance error: API key or secret key is empty")
		return false
	}

	// Синхронізація часу з сервером Binance
	serverTime, err := getServerTime()
	if err != nil {
		log.Printf("Binance error fetching server time: %v", err)
		return false
	}
	// log.Printf("Binance: Server time synchronized: %v", serverTime)

	// Додаємо timestamp до запиту
	timestamp := serverTime.UnixMilli()
	queryString := fmt.Sprintf("timestamp=%d", timestamp)

	// Генеруємо signature
	signature := generateSignature(queryString, secretKey)
	urlWithSignature := fmt.Sprintf("%s?%s&signature=%s", assetDetailURL, queryString, signature)

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", urlWithSignature, nil)
	if err != nil {
		log.Printf("Binance error creating request: %v", err)
		return false
	}
	req.Header.Set("X-MBX-APIKEY", apiKey)

	// Log API key for debugging (only first few characters for security)
	// log.Printf("Binance: Using API key: %s...", apiKey[:5])

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Binance error fetching asset details: %v", err)
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("Binance non-OK status code %d from %s", resp.StatusCode, urlWithSignature)
		return false
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Binance error reading response: %v", err)
		return false
	}

	var assets []AssetDetail
	if err := json.Unmarshal(body, &assets); err != nil {
		log.Printf("Binance error unmarshalling JSON: %v", err)
		return false
	}

	tx, err := db.Begin()
	if err != nil {
		log.Printf("Binance Failed to begin transaction: %v", err)
		return false
	}

	// Формуємо SQL-запит з `ON CONFLICT`
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

	for _, asset := range assets {
		for _, network := range asset.NetworkList {
			coinKey := fmt.Sprintf("%s_Binance_%s", asset.Coin, network.Network)
			values = append(values, fmt.Sprintf("($%d, $%d, 'Binance', $%d, $%d, $%d, $%d, $%d)", counter, counter+1, counter+2, counter+3, counter+4, counter+5, counter+6))
			args = append(args, coinKey, asset.Coin, network.Network, network.Name, network.DepositEnable, network.WithdrawEnable, time.Now().UTC())
			counter += 7
		}
	}

	if len(values) == 0 {
		log.Println("Binance: No network data to update")
		tx.Rollback()
		return true
	}

	fullQuery := fmt.Sprintf(query, strings.Join(values, ", "))
	_, err = tx.Exec(fullQuery, args...)
	if err != nil {
		tx.Rollback()
		log.Printf("Binance Failed to execute statement: %v", err)
		return false
	}

	if err := tx.Commit(); err != nil {
		log.Printf("Binance Failed to commit transaction: %v", err)
		return false
	}

	// log.Println("Binance: Successfully updated network availability")
	return true
}

func generateSignature(message, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(message))
	return fmt.Sprintf("%x", mac.Sum(nil))
}

func getServerTime() (time.Time, error) {
	resp, err := http.Get(serverTimeURL)
	if err != nil {
		return time.Time{}, fmt.Errorf("error fetching server time: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return time.Time{}, fmt.Errorf("non-OK status code %d from %s", resp.StatusCode, serverTimeURL)
	}

	var result struct {
		ServerTime int64 `json:"serverTime"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return time.Time{}, fmt.Errorf("error decoding server time response: %w", err)
	}

	return time.UnixMilli(result.ServerTime), nil
}

func UpdateAllFuturesPairs(db *sql.DB) bool {
	var wg sync.WaitGroup
	errChan := make(chan error, 3)

	// Variables to store data from endpoints
	var futuresExchangeInfo FuturesExchangeInfoResponse
	var futuresData []struct {
		Symbol               string `json:"symbol"`
		MarkPrice            string `json:"markPrice"`
		IndexPrice           string `json:"indexPrice"`
		FundingRate          string `json:"lastFundingRate"`
		NextFundingTimestamp int64  `json:"nextFundingTime"`
	}
	var ticker24hrFutures []struct {
		Symbol                string `json:"symbol"`
		PriceChangePercent24h string `json:"priceChangePercent"`
		BaseVolume24h         string `json:"volume"`
		QuoteVolume24h        string `json:"quoteVolume"`
	}

	// Fetch data from the Binance futures endpoints
	wg.Add(3)
	go fetchJSON(exchangeInfoFuturesURL, &futuresExchangeInfo, &wg, errChan)
	go fetchJSON(futuresDataURL, &futuresData, &wg, errChan)
	go fetchJSON(ticker24hrFuturesURL, &ticker24hrFutures, &wg, errChan)

	wg.Wait()
	close(errChan)

	// Check for errors
	for err := range errChan {
		if err != nil {
			log.Printf("Binance Error: %v", err)
			return false
		}
	}

	// Create maps for quick access to data
	symbolInfoMap := make(map[string]struct {
		BaseAsset   string
		QuoteAsset  string
		DisplayName string
	})
	for _, sym := range futuresExchangeInfo.Symbols {
		symbolInfoMap[sym.Symbol] = struct {
			BaseAsset   string
			QuoteAsset  string
			DisplayName string
		}{
			BaseAsset:   sym.BaseAsset,
			QuoteAsset:  sym.QuoteAsset,
			DisplayName: fmt.Sprintf("%s/%s", sym.BaseAsset, sym.QuoteAsset),
		}
	}

	ticker24hrMap := make(map[string]struct {
		PriceChangePercent24h string
		BaseVolume24h         string
		QuoteVolume24h        string
	})
	for _, t := range ticker24hrFutures {
		ticker24hrMap[t.Symbol] = struct {
			PriceChangePercent24h string
			BaseVolume24h         string
			QuoteVolume24h        string
		}{
			PriceChangePercent24h: t.PriceChangePercent24h,
			BaseVolume24h:         t.BaseVolume24h,
			QuoteVolume24h:        t.QuoteVolume24h,
		}
	}

	// Prepare pairs for database insertion
	var pairs []models.PairFutures
	for _, data := range futuresData {
		symbolInfo, exists := symbolInfoMap[data.Symbol]
		if !exists {
			// log.Printf("Binance Warning: Symbol %s not found in exchangeInfoFuturesURL", data.Symbol)
			continue
		}

		ticker24hr, exists := ticker24hrMap[data.Symbol]
		if !exists {
			// log.Printf("Binance Warning: Symbol %s not found in ticker24hrFuturesURL", data.Symbol)
			continue
		}

		// Parse and sanitize data
		markPrice := parseFloat(data.MarkPrice, "futuresData.MarkPrice")
		indexPrice := parseFloat(data.IndexPrice, "futuresData.IndexPrice")
		lastFundingRate := parseFloat(data.FundingRate, "futuresData.FundingRate")
		// fundingRatePercent := lastFundingRate * 100
		fundingRatePercent := lastFundingRate
		priceChangePercent24h := parseFloat(ticker24hr.PriceChangePercent24h, "ticker24hr.PriceChangePercent24h")
		baseVolume24h := parseFloat(ticker24hr.BaseVolume24h, "ticker24hr.BaseVolume24h")
		quoteVolume24h := parseFloat(ticker24hr.QuoteVolume24h, "ticker24hr.QuoteVolume24h")

		// Skip invalid data
		if markPrice <= 0 || indexPrice <= 0 {
			log.Printf("Binance Warning: Skipping invalid data for symbol %s", data.Symbol)
			continue
		}

		// Create PairFutures object
		pair := models.PairFutures{
			PairKey:               fmt.Sprintf("%s_Binance_futures", data.Symbol),
			Symbol:                data.Symbol,
			Exchange:              "Binance",
			Market:                "futures",
			MarkPrice:             markPrice,
			IndexPrice:            indexPrice,
			BaseAsset:             symbolInfo.BaseAsset,
			QuoteAsset:            symbolInfo.QuoteAsset,
			DisplayName:           symbolInfo.DisplayName,
			FundingRatePercent:    fundingRatePercent,
			NextFundingTimestamp:  int(data.NextFundingTimestamp),
			PriceChangePercent24h: priceChangePercent24h,
			BaseVolume24h:         baseVolume24h,
			QuoteVolume24h:        quoteVolume24h,
			UpdatedAt:             time.Now(),
		}
		pairs = append(pairs, pair)
	}

	// Insert pairs into the database
	if len(pairs) == 0 {
		log.Printf("Binance No futures pairs to update")
		return false
	}

	tx, err := db.Begin()
	if err != nil {
		log.Printf("Binance Failed to begin transaction: %v", err)
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
        pricechangepercent24h = EXCLUDED.pricechangepercent24h,
        basevolume24h = EXCLUDED.basevolume24h,
        quotevolume24h = EXCLUDED.quotevolume24h,
        updatedat = EXCLUDED.updatedat
    `
	stmt, err := tx.Prepare(query)
	if err != nil {
		log.Printf("Binance Failed to prepare statement: %v", err)
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
		log.Printf("Binance Failed to execute statement: %v", err)
		return false
	}

	if err := tx.Commit(); err != nil {
		log.Printf("Binance Failed to commit transaction: %v", err)
		return false
	}

	return true
}
