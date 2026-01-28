package huobi

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
	symbolsURL     = "https://api.huobi.pro/v1/common/symbols"
	tickerPriceURL = "https://api.huobi.pro/market/tickers"
	ticker24hrURL  = "https://api.huobi.pro/market/detail"
	currenciesURL  = "https://api.huobi.pro/v2/reference/currencies"

	// Обмеження для числових полів в PostgreSQL
	MAX_DECIMAL_18_8 = 9999999999.99999999   // Максимальне значення для DECIMAL(18,8)
	MAX_DECIMAL_10_2 = 99999999.99           // Максимальне значення для DECIMAL(10,2)
	MAX_DECIMAL_20_2 = 999999999999999999.99 // Максимальне значення для DECIMAL(20,2)
)

// SymbolsResponse представляє відповідь API з інформацією про символи
type SymbolsResponse struct {
	Status string `json:"status"`
	Data   []struct {
		Symbol          string `json:"symbol"`
		BaseCurrency    string `json:"base-currency"`
		QuoteCurrency   string `json:"quote-currency"`
		State           string `json:"state"`
		PricePrecision  int    `json:"price-precision"`
		AmountPrecision int    `json:"amount-precision"`
	} `json:"data"`
}

// TickersResponse представляє відповідь API з цінами
type TickersResponse struct {
	Status string `json:"status"`
	Data   []struct {
		Symbol string  `json:"symbol"`
		Open   float64 `json:"open"`
		Close  float64 `json:"close"`
		Amount float64 `json:"amount"`
		Vol    float64 `json:"vol"`
	} `json:"data"`
}

// DetailResponse представляє детальну інформацію про ринок
type DetailResponse struct {
	Status string `json:"status"`
	Ch     string `json:"ch"`
	Data   struct {
		Amount float64 `json:"amount"`
		Open   float64 `json:"open"`
		Close  float64 `json:"close"`
		Volume float64 `json:"vol"`
	} `json:"data"`
}

// Структура для парсингу відповіді API
type CurrenciesResponse struct {
	Code int `json:"code"`
	Data []struct {
		Currency string `json:"currency"`
		Chains   []struct {
			Chain          string `json:"chain"`
			DepositStatus  string `json:"depositStatus"`
			WithdrawStatus string `json:"withdrawStatus"`
			FullName       string `json:"displayName"`
			Name           string `json:"fullName"`
		} `json:"chains"`
	} `json:"data"`
}

// fetchJSON універсальна функція для отримання JSON з API
func fetchJSON(url string, target interface{}, wg *sync.WaitGroup, errChan chan<- error) {
	defer wg.Done()

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Get(url)
	if err != nil {
		errChan <- fmt.Errorf("Huobi error fetching %s: %w", url, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errChan <- fmt.Errorf("Huobi non-OK status code %d from %s", resp.StatusCode, url)
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		errChan <- fmt.Errorf("Huobi error reading response from %s: %w", url, err)
		return
	}

	if err := json.Unmarshal(body, target); err != nil {
		errChan <- fmt.Errorf("Huobi error unmarshalling JSON from %s: %w", url, err)
	}
}

// sanitizeDecimal перевіряє та обмежує числове значення
func sanitizeDecimal(value float64, maxValue float64, precision int) float64 {
	// Перевіряємо на NaN та Inf
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}

	// Обмежуємо значення
	if value > maxValue {
		// log.Printf("Warning: Value %f exceeds maximum allowed %f, clamping", value, maxValue)
		value = maxValue
	} else if value < -maxValue {
		// log.Printf("Warning: Value %f exceeds minimum allowed %f, clamping", value, -maxValue)
		value = -maxValue
	}

	// Форматуємо з потрібною точністю
	format := "%." + strconv.Itoa(precision) + "f"
	strVal := fmt.Sprintf(format, value)
	formattedVal, _ := strconv.ParseFloat(strVal, 64)
	return formattedVal
}

// calculatePercentChange розраховує відсоткову зміну
func calculatePercentChange(open, close float64) float64 {
	if open == 0 {
		return 0
	}
	return ((close - open) / open) * 100
}

// generateNumberedPlaceholders генерує SQL плейсхолдери для пакетного вставлення
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

// UpdateAllSpotPairs оновлює інформацію про всі спотові пари з Huobi
func UpdateAllSpotPairs(db *sql.DB) bool {
	var wg sync.WaitGroup
	errChan := make(chan error, 2)

	// Змінні для збереження відповідей
	var symbolsInfo SymbolsResponse
	var tickersInfo TickersResponse

	// Запускаємо два паралельні запити
	wg.Add(2)
	go fetchJSON(symbolsURL, &symbolsInfo, &wg, errChan)
	go fetchJSON(tickerPriceURL, &tickersInfo, &wg, errChan)

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

	// Перевіряємо статуси відповідей
	if symbolsInfo.Status != "ok" || tickersInfo.Status != "ok" {
		log.Printf("Huobi API returned non-OK status")
		return false
	}

	// Створюємо мапу для швидкого доступу до даних тікера
	tickerMap := make(map[string]struct {
		Close  float64
		Open   float64
		Amount float64
		Volume float64
	})

	for _, ticker := range tickersInfo.Data {
		tickerMap[ticker.Symbol] = struct {
			Close  float64
			Open   float64
			Amount float64
			Volume float64
		}{
			Close:  ticker.Close,
			Open:   ticker.Open,
			Amount: ticker.Amount,
			Volume: ticker.Vol,
		}
	}

	// Формуємо фінальний масив `Pair`
	var pairs []models.Pair
	now := time.Now()

	for _, sym := range symbolsInfo.Data {
		// Перевіряємо чи активна пара
		if sym.State != "online" {
			continue
		}

		// Перевіряємо наявність даних тікера
		tickerData, exists := tickerMap[sym.Symbol]
		if !exists {
			continue
		}

		// Розраховуємо зміну ціни за 24 години
		priceChangePercent := calculatePercentChange(tickerData.Open, tickerData.Close)

		// Санітизуємо значення з відповідними обмеженнями
		price := sanitizeDecimal(tickerData.Close, MAX_DECIMAL_18_8, 8)
		priceChangeFormatted := sanitizeDecimal(priceChangePercent, MAX_DECIMAL_10_2, 2)
		baseVolume := sanitizeDecimal(tickerData.Amount, MAX_DECIMAL_20_2, 2)
		quoteVolume := sanitizeDecimal(tickerData.Volume, MAX_DECIMAL_20_2, 2)

		// Перевірка на неприпустимі значення для полів price, baseVolume і quoteVolume
		if price <= 0 || math.IsNaN(price) || math.IsInf(price, 0) {
			// log.Printf("Skipping pair %s due to invalid price: %v", sym.Symbol, tickerData.Close)
			continue
		}

		// Приводимо назви валют до верхнього регістру для консистентності
		baseAsset := strings.ToUpper(sym.BaseCurrency)
		quoteAsset := strings.ToUpper(sym.QuoteCurrency)

		// Обмеження довжини полів
		if len(sym.Symbol) > 20 {
			sym.Symbol = sym.Symbol[:20]
		}
		if len(baseAsset) > 20 {
			baseAsset = baseAsset[:20]
		}
		if len(quoteAsset) > 20 {
			quoteAsset = quoteAsset[:20]
		}

		displayName := fmt.Sprintf("%s/%s", strings.ToUpper(baseAsset), strings.ToUpper(quoteAsset))
		if len(displayName) > 20 {
			displayName = displayName[:20]
		}

		pairKey := fmt.Sprintf("%s_HUOBI_SPOT", strings.ToUpper(sym.Symbol))
		if len(pairKey) > 50 {
			pairKey = pairKey[:50]
		}

		pair := models.Pair{
			PairKey:               pairKey,
			Symbol:                strings.ToUpper(sym.Symbol),
			Exchange:              "Huobi",
			Market:                "spot",
			Price:                 price,
			BaseAsset:             strings.ToUpper(baseAsset),
			QuoteAsset:            strings.ToUpper(quoteAsset),
			DisplayName:           displayName,
			PriceChangePercent24h: priceChangeFormatted,
			BaseVolume24h:         baseVolume,
			QuoteVolume24h:        quoteVolume,
			UpdatedAt:             now,
		}

		pairs = append(pairs, pair)
	}

	// Перевіряємо, чи є дані для вставки
	if len(pairs) == 0 {
		log.Printf("Huobi: No pairs data to insert")
		return false
	}

	// Розпочинаємо транзакцію
	tx, err := db.Begin()
	if err != nil {
		log.Printf("Huobi: Failed to begin transaction: %v", err)
		return false
	}

	// Використовуємо 12 колонок на запис
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
		tx.Rollback()
		log.Printf("Huobi: Failed to prepare statement: %v", err)
		return false
	}
	defer stmt.Close()

	// Підготовка аргументів для запиту
	args := make([]interface{}, 0, len(pairs)*12)
	for _, pair := range pairs {
		args = append(args,
			pair.PairKey,
			pair.Symbol,
			pair.Exchange,
			pair.Market,
			pair.Price,
			pair.BaseAsset,
			pair.QuoteAsset,
			pair.DisplayName,
			pair.PriceChangePercent24h,
			pair.BaseVolume24h,
			pair.QuoteVolume24h,
			pair.UpdatedAt)
	}

	// Виконання запиту
	_, err = stmt.Exec(args...)
	if err != nil {
		tx.Rollback()
		log.Printf("Huobi: Failed to execute statement: %v", err)
		return false
	}

	// Завершення транзакції
	if err := tx.Commit(); err != nil {
		log.Printf("Huobi: Failed to commit transaction: %v", err)
		return false
	}

	// log.Printf("Huobi: Successfully updated %d spot pairs", len(pairs))
	return true
}

// Функція для збору та збереження мереж із Huobi
func UpdateAllNetworks(db *sql.DB) bool {
	// Запит до API
	resp, err := http.Get(currenciesURL)
	if err != nil {
		log.Printf("error fetching data from Huobi: %v", err)
		return false
	}
	defer resp.Body.Close()

	var result CurrenciesResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Printf("error decoding response: %v", err)
		return false
	}

	// Підготовка SQL-запиту на вставку/оновлення
	query := `
		INSERT INTO nets (coinKey, coin, exchange, network, networkName, depositEnable, withdrawEnable, updatedAt)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (coinKey) DO UPDATE 
		SET depositEnable = EXCLUDED.depositEnable,
		    withdrawEnable = EXCLUDED.withdrawEnable,
		    updatedAt = EXCLUDED.updatedAt
	`

	// Обробка отриманих даних
	for _, coin := range result.Data {
		coinSymbol := strings.ToUpper(coin.Currency)

		for _, chain := range coin.Chains {
			network := strings.ToUpper(chain.Name) // Наприклад, "BTC", "BSC", "ERC20"
			networkName := chain.FullName          // Наприклад, "Bitcoin", "Binance Smart Chain"
			depositEnabled := chain.DepositStatus == "allowed"
			withdrawEnabled := chain.WithdrawStatus == "allowed"

			coinKey := fmt.Sprintf("%s_Huobi_%s", coinSymbol, network)
			updatedAt := time.Now().UTC() // Поточний час у форматі UTC

			_, err := db.Exec(query, coinKey, coinSymbol, "Huobi", network, networkName, depositEnabled, withdrawEnabled, updatedAt)
			if err != nil {
				log.Printf("Error inserting/updating %s: %v", coinKey, err)
			}
		}
	}

	// fmt.Println("Huobi networks updated successfully.")
	return true
}
