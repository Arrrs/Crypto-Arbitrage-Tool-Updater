package backpack

import (
	"crypto/ed25519"
	"database/sql"
	"encoding/base64"
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
	exchangeInfoURL = "https://api.backpack.exchange/api/v1/markets"
	ticker24hrURL   = "https://api.backpack.exchange/api/v1/tickers"
	assetDetailURL  = "https://api.backpack.exchange/api/v1/capital"
	serverTimeURL   = "https://api.backpack.exchange/api/v1/time"
	markPricesURL   = "https://api.backpack.exchange/api/v1/markPrices"
)

// Структура для відповіді про торгові пари
type ExchangeInfoResponse []struct {
	Symbol     string `json:"symbol"`
	BaseAsset  string `json:"baseSymbol"`
	QuoteAsset string `json:"quoteSymbol"`
	Type       string `json:"marketType"`
}

// Структура для статистики за 24 години
type Ticker24hrResponse struct {
	Symbol                string `json:"symbol"`
	Price                 string `json:"lastPrice"`
	PriceChangePercent24h string `json:"priceChangePercent"`
	Volume                string `json:"volume"`      // Базовий обсяг
	QuoteVolume           string `json:"quoteVolume"` // Квотовий обсяг
}

// Структура для доступних мереж
type AssetDetail struct {
	Asset    string `json:"asset"`
	Networks []struct {
		Network           string `json:"network"`
		Name              string `json:"name"`
		DepositEnabled    bool   `json:"depositEnabled"`
		WithdrawalEnabled bool   `json:"withdrawalEnabled"`
	} `json:"networks"`
}

type MarkPrices struct {
	Symbol               string `json:"symbol"`
	FundingRate          string `json:"fundingRate"`
	IndexPrice           string `json:"indexPrice"`
	MarkPrice            string `json:"markPrice"`
	NextFundingTimestamp int64  `json:"nextFundingTimestamp"`
}

// Функція для виконання HTTP-запиту та парсингу JSON
func fetchJSON(url string, target interface{}, wg *sync.WaitGroup, errChan chan<- error) {
	defer wg.Done()

	resp, err := http.Get(url)
	if err != nil {
		errChan <- fmt.Errorf("Backpack error fetching %s: %w", url, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errChan <- fmt.Errorf("Backpack non-OK status code %d from %s", resp.StatusCode, url)
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		errChan <- fmt.Errorf("Backpack error reading response from %s: %w", url, err)
		return
	}

	if err := json.Unmarshal(body, target); err != nil {
		errChan <- fmt.Errorf("Backpack error unmarshalling JSON from %s: %w", url, err)
	}
}

// parseFloat - конвертація рядка в float64 (returns 0, false for empty/invalid strings)
func parseFloat(s string, d string) (float64, bool) {
	if s == "" {
		return 0, false
	}
	val, err := strconv.ParseFloat(s, 64)
	if err != nil {
		log.Printf("Backpack Warning: failed to parse float from %s, description: %s", s, d)
		return 0, false
	}
	return val, true
}

// formatFloat - форматування float64 з заданою точністю
func formatFloat(val float64, precision int) float64 {
	format := "%." + strconv.Itoa(precision) + "f"
	strVal := fmt.Sprintf(format, val)
	formattedVal, _ := strconv.ParseFloat(strVal, 64)
	return formattedVal
}

// generateNumberedPlaceholders - генерація плейсхолдерів для SQL-запиту
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

// UpdateAllSpotPairs - оновлення даних про торгові пари
func UpdateAllSpotPairs(db *sql.DB) bool {
	// log.Printf("Backpack update pairs started")
	var wg sync.WaitGroup
	errChan := make(chan error, 3)

	// Змінні для збереження відповідей
	var exchangeInfo ExchangeInfoResponse
	var ticker24hrs []Ticker24hrResponse

	// Запускаємо три паралельні запити
	wg.Add(2)
	go fetchJSON(exchangeInfoURL, &exchangeInfo, &wg, errChan)
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

	ticker24hrMap := make(map[string]Ticker24hrResponse)
	for _, t := range ticker24hrs {
		ticker24hrMap[t.Symbol] = t
	}

	// Формуємо масив `Pair`
	var pairs []models.Pair
	for _, market := range exchangeInfo {
		if market.Type != "SPOT" {
			continue
		}

		ticker24hr := ticker24hrMap[market.Symbol]

		// Skip pairs with empty/invalid price data
		price, ok := parseFloat(ticker24hr.Price, "")
		if !ok {
			continue
		}

		priceChange, _ := parseFloat(ticker24hr.PriceChangePercent24h, "")
		baseVolume, _ := parseFloat(ticker24hr.Volume, "")
		quoteVolume, _ := parseFloat(ticker24hr.QuoteVolume, "")

		pair := models.Pair{
			PairKey:               fmt.Sprintf("%s_Backpack_spot", strings.ReplaceAll(market.Symbol, "_", "")),
			Symbol:                strings.ReplaceAll(market.Symbol, "_", ""),
			Exchange:              "Backpack",
			Market:                "spot",
			Price:                 formatFloat(price, 8),
			BaseAsset:             market.BaseAsset,
			QuoteAsset:            market.QuoteAsset,
			DisplayName:           fmt.Sprintf("%s/%s", market.BaseAsset, market.QuoteAsset),
			PriceChangePercent24h: formatFloat(priceChange, 2),
			BaseVolume24h:         formatFloat(baseVolume, 2),
			QuoteVolume24h:        formatFloat(quoteVolume, 2),
			UpdatedAt:             time.Now(),
		}
		pairs = append(pairs, pair)
	}

	// Зберігаємо в базу даних
	tx, err := db.Begin()
	if err != nil {
		log.Printf("Backpack Failed to begin transaction: %v", err)
		return false
	}

	// Використовуємо 12 колонок для запису
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
		log.Printf("Backpack Failed to prepare statement: %v", err)
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
		log.Printf("Backpack Failed to execute statement: %v", err)
		return false
	}

	if err := tx.Commit(); err != nil {
		log.Printf("Backpack Failed to commit transaction: %v", err)
		return false
	}

	// log.Printf("Backpack pairs updated")
	return true
}

// UpdateAllNetworks - оновлення даних про доступні мережі
func UpdateAllNetworks(db *sql.DB, apiKey, secretKey string) bool {
	if apiKey == "" || secretKey == "" {
		log.Println("Backpack error: API key or secret key is empty")
		return false
	}

	// Синхронізація часу з сервером Backpack
	serverTime, err := getServerTime()
	if err != nil {
		log.Printf("Backpack error fetching server time: %v", err)
		return false
	}

	// Додаємо timestamp і window до запиту
	timestamp := serverTime.UnixMilli()
	receiveWindow := 5000 // Рекомендоване значення
	queryString := fmt.Sprintf("timestamp=%d&window=%d", timestamp, receiveWindow)

	// Генеруємо підпис (Backpack використовує ED25519)
	signature := generateSignature(queryString, secretKey)
	urlWithSignature := fmt.Sprintf("%s?%s", assetDetailURL, queryString)

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", urlWithSignature, nil)
	if err != nil {
		log.Printf("Backpack error creating request: %v", err)
		return false
	}
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("X-Signature", signature)
	req.Header.Set("X-Timestamp", fmt.Sprintf("%d", timestamp))
	req.Header.Set("X-Window", fmt.Sprintf("%d", receiveWindow))

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Backpack error fetching asset details: %v", err)
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("Backpack non-OK status code %d from %s", resp.StatusCode, urlWithSignature)
		return false
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Backpack error reading response: %v", err)
		return false
	}

	var assets []AssetDetail
	if err := json.Unmarshal(body, &assets); err != nil {
		log.Printf("Backpack error unmarshalling JSON: %v", err)
		return false
	}

	tx, err := db.Begin()
	if err != nil {
		log.Printf("Backpack Failed to begin transaction: %v", err)
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
		for _, network := range asset.Networks {
			coinKey := fmt.Sprintf("%s_Backpack_%s", asset.Asset, network.Network)
			values = append(values, fmt.Sprintf("($%d, $%d, 'Backpack', $%d, $%d, $%d, $%d, $%d)", counter, counter+1, counter+2, counter+3, counter+4, counter+5, counter+6))
			args = append(args, coinKey, asset.Asset, network.Network, network.Name, network.DepositEnabled, network.WithdrawalEnabled, time.Now().UTC())
			counter += 7
		}
	}

	if len(values) == 0 {
		log.Println("Backpack: No network data to update")
		tx.Rollback()
		return true
	}

	fullQuery := fmt.Sprintf(query, strings.Join(values, ", "))
	_, err = tx.Exec(fullQuery, args...)
	if err != nil {
		tx.Rollback()
		log.Printf("Backpack Failed to execute statement: %v", err)
		return false
	}

	if err := tx.Commit(); err != nil {
		log.Printf("Backpack Failed to commit transaction: %v", err)
		return false
	}

	return true
}

// generateSignature - генерація ED25519 підпису
func generateSignature(message, secretKey string) string {
	// Backpack використовує ED25519, а не HMAC-SHA256, як Binance
	secretBytes, err := base64.StdEncoding.DecodeString(secretKey)
	if err != nil {
		log.Printf("Backpack error decoding secret key: %v", err)
		return ""
	}

	// Перевіряємо, чи ключ відповідає ED25519 (64 байти для приватного ключа)
	if len(secretBytes) != ed25519.PrivateKeySize {
		log.Printf("Backpack error: invalid ED25519 secret key length")
		return ""
	}

	privateKey := ed25519.PrivateKey(secretBytes)
	signature := ed25519.Sign(privateKey, []byte(message))
	return base64.StdEncoding.EncodeToString(signature)
}

// getServerTime - отримання часу сервера Backpack
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
	// log.Printf("Backpack update pairs started")
	var wg sync.WaitGroup
	errChan := make(chan error, 3)

	// Змінні для збереження відповідей
	var exchangeInfo ExchangeInfoResponse
	var ticker24hrs []Ticker24hrResponse
	var markPrices []MarkPrices

	// Запускаємо три паралельні запити
	wg.Add(3)
	go fetchJSON(exchangeInfoURL, &exchangeInfo, &wg, errChan)
	go fetchJSON(ticker24hrURL, &ticker24hrs, &wg, errChan)
	go fetchJSON(markPricesURL, &markPrices, &wg, errChan)

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

	ticker24hrMap := make(map[string]Ticker24hrResponse)
	for _, t := range ticker24hrs {
		ticker24hrMap[t.Symbol] = t
	}

	markPricesMap := make(map[string]MarkPrices)
	for _, d := range markPrices {
		markPricesMap[d.Symbol] = d
	}

	// Формуємо масив `Pair`
	var pairs []models.PairFutures
	for _, market := range exchangeInfo {
		if market.Type != "PERP" {
			continue
		}

		markPricesTemp := markPricesMap[market.Symbol]
		ticker24hr := ticker24hrMap[market.Symbol]

		// Skip pairs with empty/invalid mark price data
		markprice, ok := parseFloat(markPricesTemp.MarkPrice, "")
		if !ok {
			continue
		}
		indexprice, _ := parseFloat(markPricesTemp.IndexPrice, "")
		fundingRate, _ := parseFloat(markPricesTemp.FundingRate, "")
		priceChange, _ := parseFloat(ticker24hr.PriceChangePercent24h, "")
		baseVolume, _ := parseFloat(ticker24hr.Volume, "")
		quoteVolume, _ := parseFloat(ticker24hr.QuoteVolume, "")

		symbol := strings.ReplaceAll(strings.ReplaceAll(market.Symbol, "PERP", ""), "_", "")

		pair := models.PairFutures{
			PairKey:               fmt.Sprintf("%s_Backpack_futures", symbol),
			Symbol:                symbol,
			Exchange:              "Backpack",
			Market:                "futures",
			MarkPrice:             formatFloat(markprice, 8),
			IndexPrice:            formatFloat(indexprice, 8),
			BaseAsset:             market.BaseAsset,
			QuoteAsset:            market.QuoteAsset,
			DisplayName:           fmt.Sprintf("%s/%s", market.BaseAsset, market.QuoteAsset),
			FundingRatePercent:    formatFloat(fundingRate, 6),
			NextFundingTimestamp:  int(markPricesTemp.NextFundingTimestamp / 1000), // Convert milliseconds to seconds
			PriceChangePercent24h: formatFloat(priceChange, 2),
			BaseVolume24h:         formatFloat(baseVolume, 2),
			QuoteVolume24h:        formatFloat(quoteVolume, 2),
			UpdatedAt:             time.Now(),
		}
		pairs = append(pairs, pair)
	}

	// Зберігаємо в базу даних
	tx, err := db.Begin()
	if err != nil {
		log.Printf("Backpack Failed to begin transaction: %v", err)
		return false
	}

	// Використовуємо 15 колонок для запису
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
		log.Printf("Backpack Failed to prepare statement: %v", err)
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
			pair.NextFundingTimestamp, // Ensure this field is included
			pair.PriceChangePercent24h,
			pair.BaseVolume24h,
			pair.QuoteVolume24h,
			pair.UpdatedAt,
		)
	}

	_, err = stmt.Exec(args...)
	if err != nil {
		tx.Rollback()
		log.Printf("Backpack Failed to execute statement: %v", err)
		return false
	}

	if err := tx.Commit(); err != nil {
		log.Printf("Backpack Failed to commit transaction: %v", err)
		return false
	}

	// log.Printf("Backpack pairs futures updated")
	return true
}
