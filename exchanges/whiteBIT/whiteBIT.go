package whitebit

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
	marketsURL = "https://whitebit.com/api/v4/public/markets"
	tickerURL  = "https://whitebit.com/api/v4/public/ticker"
	// networksURL      = "https://whitebit.com/api/v4/public/coins"
	assetsURL        = "https://whitebit.com/api/v4/public/assets"
	MAX_DECIMAL_18_8 = 9999999999.99999999   // Максимальне значення для DECIMAL(18,8)
	MAX_DECIMAL_10_2 = 99999999.99           // Максимальне значення для DECIMAL(10,2)
	MAX_DECIMAL_20_2 = 999999999999999999.99 // Максимальне значення для DECIMAL(20,2)
)

type MarketInfo struct {
	Name          string `json:"name"`
	BaseAsset     string `json:"stock"`
	QuoteAsset    string `json:"money"`
	TradesEnabled bool   `json:"tradesEnabled"`
}

type TickerInfo struct {
	Symbol    string `json:"symbol"`
	LastPrice string `json:"last_price"`
	Volume    string `json:"base_volume"`
	Change24h string `json:"change"`
}

// type AssetInfo struct {
// 	Name     string `json:"name"`
// 	Networks []struct {
// 		NetworkName    string `json:"protocol"`
// 		DepositEnable  bool   `json:"deposit_enabled"`
// 		WithdrawEnable bool   `json:"withdraw_enabled"`
// 	} `json:"networks"`
// }

type AssetInfo struct {
	Name              string `json:"name"`
	MinWithdraw       string `json:"min_withdraw"`
	MinDeposit        string `json:"min_deposit"`
	CurrencyPrecision int    `json:"currency_precision"`
	Networks          struct {
		Deposits  []string `json:"deposits"`
		Withdraws []string `json:"withdraws"`
		Default   string   `json:"default"`
	} `json:"networks"`
	Limits struct {
		Deposit  map[string]map[string]string `json:"deposit"`
		Withdraw map[string]map[string]string `json:"withdraw"`
	} `json:"limits"`
}

func fetchJSON(url string, target interface{}, wg *sync.WaitGroup, errChan chan<- error) {
	defer wg.Done()

	resp, err := http.Get(url)
	if err != nil {
		errChan <- fmt.Errorf("WhiteBIT error fetching %s: %w", url, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errChan <- fmt.Errorf("WhiteBIT non-OK status code %d from %s", resp.StatusCode, url)
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		errChan <- fmt.Errorf("WhiteBIT error reading response from %s: %w", url, err)
		return
	}

	if err := json.Unmarshal(body, target); err != nil {
		errChan <- fmt.Errorf("WhiteBIT error unmarshalling JSON from %s: %w", url, err)
	}
}

func parseTickerJSON(body []byte) (map[string]TickerInfo, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("WhiteBIT error unmarshalling ticker JSON: %w", err)
	}

	tickerMap := make(map[string]TickerInfo)
	for symbol, data := range raw {
		var ticker TickerInfo
		if err := json.Unmarshal(data, &ticker); err != nil {
			return nil, fmt.Errorf("WhiteBIT error unmarshalling ticker data for %s: %w", symbol, err)
		}
		ticker.Symbol = symbol
		tickerMap[symbol] = ticker
	}

	return tickerMap, nil
}

func parseFloat(s string) float64 {
	val, err := strconv.ParseFloat(s, 64)
	if err != nil {
		log.Printf("WhiteBIT Warning: failed to parse float from %s", s)
		return 0
	}
	return val
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

func UpdateAllSpotPairs(db *sql.DB) bool {
	var wg sync.WaitGroup
	errChan := make(chan error, 3)

	var markets []MarketInfo
	var tickers map[string]TickerInfo

	wg.Add(1)
	go fetchJSON(marketsURL, &markets, &wg, errChan)

	resp, err := http.Get(tickerURL)
	if err != nil {
		log.Printf("WhiteBIT error fetching %s: %v", tickerURL, err)
		return false
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("WhiteBIT error reading response from %s: %v", tickerURL, err)
		return false
	}

	tickers, err = parseTickerJSON(body)
	if err != nil {
		log.Printf("WhiteBIT error parsing ticker JSON: %v", err)
		return false
	}

	wg.Wait()
	close(errChan)

	for err := range errChan {
		if err != nil {
			log.Printf("Error: %v", err)
			return false
		}
	}

	var pairs []models.Pair
	for _, market := range markets {
		if !market.TradesEnabled {
			continue
		}

		ticker, exists := tickers[market.Name]
		if !exists {
			continue
		}

		price := sanitizeDecimal(parseFloat(ticker.LastPrice), MAX_DECIMAL_18_8, 8)
		priceChangePercent := sanitizeDecimal(parseFloat(ticker.Change24h), MAX_DECIMAL_10_2, 2)
		baseVolume := sanitizeDecimal(parseFloat(ticker.Volume), MAX_DECIMAL_20_2, 2)
		quoteVolume := sanitizeDecimal(parseFloat(ticker.Volume)*parseFloat(ticker.LastPrice), MAX_DECIMAL_20_2, 2)

		if price <= 0 || math.IsNaN(price) || math.IsInf(price, 0) {
			continue
		}

		pair := models.Pair{
			PairKey:               fmt.Sprintf("%s_WhiteBIT_spot", strings.ReplaceAll(market.Name, "_", "")),
			Symbol:                strings.ReplaceAll(market.Name, "_", ""),
			Exchange:              "WhiteBIT",
			Market:                "spot",
			Price:                 price,
			BaseAsset:             market.BaseAsset,
			QuoteAsset:            market.QuoteAsset,
			DisplayName:           fmt.Sprintf("%s/%s", market.BaseAsset, market.QuoteAsset),
			PriceChangePercent24h: priceChangePercent,
			BaseVolume24h:         baseVolume,
			QuoteVolume24h:        quoteVolume,
			UpdatedAt:             time.Now(),
		}
		pairs = append(pairs, pair)
	}

	tx, err := db.Begin()
	if err != nil {
		log.Printf("WhiteBIT Failed to begin transaction: %v", err)
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
		log.Printf("WhiteBIT Failed to prepare statement: %v", err)
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
		log.Printf("WhiteBIT Failed to execute statement: %v", err)
		return false
	}

	if err := tx.Commit(); err != nil {
		log.Printf("WhiteBIT Failed to commit transaction: %v", err)
		return false
	}

	return true
}

func UpdateAllNetworks(db *sql.DB) bool {
	var wg sync.WaitGroup
	errChan := make(chan error, 1)
	assets := make(map[string]AssetInfo)

	wg.Add(1)
	go fetchJSON(assetsURL, &assets, &wg, errChan)
	wg.Wait()
	close(errChan)

	for err := range errChan {
		if err != nil {
			log.Printf("Error fetching WhiteBIT data: %v", err)
			return false
		}
	}

	if len(assets) == 0 {
		log.Println("WhiteBIT: No asset data received.")
		return false
	}

	tx, err := db.Begin()
	if err != nil {
		log.Printf("WhiteBIT Failed to begin transaction: %v", err)
		return false
	}

	_, err = tx.Exec(`DELETE FROM nets WHERE exchange = 'WhiteBIT'`)
	if err != nil {
		tx.Rollback()
		log.Printf("WhiteBIT Failed to delete old network records: %v", err)
		return false
	}

	var nets []struct {
		CoinKey        string
		Coin           string
		Exchange       string
		Network        string
		NetworkName    string
		DepositEnable  bool
		WithdrawEnable bool
		UpdatedAt      time.Time
	}

	for coin, asset := range assets {
		networkMap := make(map[string]struct {
			DepositEnable  bool
			WithdrawEnable bool
		})

		// Обробка депозитів
		for _, net := range asset.Networks.Deposits {
			if entry, exists := networkMap[net]; exists {
				entry.DepositEnable = true
				networkMap[net] = entry
			} else {
				networkMap[net] = struct {
					DepositEnable  bool
					WithdrawEnable bool
				}{DepositEnable: true, WithdrawEnable: false}
			}
		}

		// Обробка виводів
		for _, net := range asset.Networks.Withdraws {
			if entry, exists := networkMap[net]; exists {
				entry.WithdrawEnable = true
				networkMap[net] = entry
			} else {
				networkMap[net] = struct {
					DepositEnable  bool
					WithdrawEnable bool
				}{DepositEnable: false, WithdrawEnable: true}
			}
		}

		// Формування списку записів
		for network, data := range networkMap {
			nets = append(nets, struct {
				CoinKey        string
				Coin           string
				Exchange       string
				Network        string
				NetworkName    string
				DepositEnable  bool
				WithdrawEnable bool
				UpdatedAt      time.Time
			}{
				CoinKey:        fmt.Sprintf("%s_WhiteBIT_%s", coin, network),
				Coin:           coin,
				Exchange:       "WhiteBIT",
				Network:        network,
				NetworkName:    network,
				DepositEnable:  data.DepositEnable,
				WithdrawEnable: data.WithdrawEnable,
				UpdatedAt:      time.Now().UTC(),
			})
		}
	}

	if len(nets) == 0 {
		log.Println("WhiteBIT: No valid network entries to update.")
		tx.Commit()
		return false
	}

	// Формуємо INSERT-запит з ON CONFLICT
	query := `
		INSERT INTO nets (coinkey, coin, exchange, network, networkname, depositenable, withdrawenable, updatedat)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (coinkey) DO UPDATE SET
		depositenable = EXCLUDED.depositenable,
		withdrawenable = EXCLUDED.withdrawenable,
		updatedat = EXCLUDED.updatedat;
	`

	stmt, err := tx.Prepare(query)
	if err != nil {
		log.Printf("WhiteBIT Failed to prepare statement: %v", err)
		tx.Rollback()
		return false
	}
	defer stmt.Close()

	// Вставка записів у базу
	for _, net := range nets {
		_, err = stmt.Exec(net.CoinKey, net.Coin, net.Exchange, net.Network, net.NetworkName, net.DepositEnable, net.WithdrawEnable, net.UpdatedAt)
		if err != nil {
			tx.Rollback()
			log.Printf("WhiteBIT Failed to execute statement: %v", err)
			return false
		}
	}

	if err := tx.Commit(); err != nil {
		log.Printf("WhiteBIT Failed to commit transaction: %v", err)
		return false
	}

	// log.Printf("WhiteBIT: Updated %d network entries.", len(nets))
	return true
}
