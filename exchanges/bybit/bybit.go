package bybit

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
	symbolsURL        = "https://api.bybit.com/v5/market/instruments-info?category=spot"
	symbolsFuturesURL = "https://api.bybit.com/v5/market/instruments-info?category=linear"
	tickerURL         = "https://api.bybit.com/v5/market/tickers?category=spot"
	tickerFuturesURL  = "https://api.bybit.com/v5/market/tickers?category=linear"
)

type SymbolsResponse struct {
	Result struct {
		List []struct {
			Symbol     string `json:"symbol"`
			BaseAsset  string `json:"baseCoin"`
			QuoteAsset string `json:"quoteCoin"`
		} `json:"list"`
	} `json:"result"`
}

type TickerResponse struct {
	Result struct {
		List []struct {
			Symbol         string `json:"symbol"`
			LastPrice      string `json:"lastPrice"`
			PriceChange24h string `json:"price24hPcnt"`
			BaseVolume24h  string `json:"volume24h"`
			QuoteVolume24h string `json:"turnover24h"`
		} `json:"list"`
	} `json:"result"`
}
type TickerResponseFutures struct {
	Result struct {
		List []struct {
			Symbol         string `json:"symbol"`
			LastPrice      string `json:"lastPrice"`
			PriceChange24h string `json:"price24hPcnt"`
			BaseVolume24h  string `json:"volume24h"`
			QuoteVolume24h string `json:"turnover24h"`
			FundingRate    string `json:"fundingRate"`
		} `json:"list"`
	} `json:"result"`
}

func fetchJSON(url string, target interface{}, wg *sync.WaitGroup, errChan chan<- error) {
	defer wg.Done()
	resp, err := http.Get(url)
	if err != nil {
		errChan <- fmt.Errorf("error fetching %s: %w", url, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errChan <- fmt.Errorf("non-OK status code %d from %s", resp.StatusCode, url)
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		errChan <- fmt.Errorf("error reading response from %s: %w", url, err)
		return
	}

	if err := json.Unmarshal(body, target); err != nil {
		errChan <- fmt.Errorf("error unmarshalling JSON from %s: %w", url, err)
	}
}

func parseFloat(s string, d string) float64 {
	val, err := strconv.ParseFloat(s, 64)
	if err != nil {
		log.Printf("Bybit Warning: failed to parse float from %s, description: %s", s, d)
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

func UpdateAllSpotPairs(db *sql.DB) bool {
	var wg sync.WaitGroup
	errChan := make(chan error, 2)

	var symbols SymbolsResponse
	var tickers TickerResponse

	wg.Add(2)
	go fetchJSON(symbolsURL, &symbols, &wg, errChan)
	go fetchJSON(tickerURL, &tickers, &wg, errChan)

	wg.Wait()
	close(errChan)

	for err := range errChan {
		if err != nil {
			log.Printf("Error: %v", err)
			return false
		}
	}

	tickerMap := make(map[string]struct {
		Symbol         string `json:"symbol"`
		LastPrice      string `json:"lastPrice"`
		PriceChange24h string `json:"price24hPcnt"`
		BaseVolume24h  string `json:"volume24h"`
		QuoteVolume24h string `json:"turnover24h"`
	})
	for _, t := range tickers.Result.List {
		tickerMap[t.Symbol] = struct {
			Symbol         string `json:"symbol"`
			LastPrice      string `json:"lastPrice"`
			PriceChange24h string `json:"price24hPcnt"`
			BaseVolume24h  string `json:"volume24h"`
			QuoteVolume24h string `json:"turnover24h"`
		}{
			Symbol:         t.Symbol,
			LastPrice:      t.LastPrice,
			PriceChange24h: t.PriceChange24h,
			BaseVolume24h:  t.BaseVolume24h,
			QuoteVolume24h: t.QuoteVolume24h,
		}
	}

	var pairs []models.Pair
	for _, sym := range symbols.Result.List {
		ticker, exists := tickerMap[sym.Symbol]
		if !exists {
			continue
		}

		pair := models.Pair{
			PairKey:               fmt.Sprintf("%s_Bybit_spot", sym.Symbol),
			Symbol:                sym.Symbol,
			Exchange:              "Bybit",
			Market:                "spot",
			Price:                 parseFloat(ticker.LastPrice, "UpdateAllSpotPairs: parsing LastPrice"),
			BaseAsset:             sym.BaseAsset,
			QuoteAsset:            sym.QuoteAsset,
			DisplayName:           fmt.Sprintf("%s/%s", sym.BaseAsset, sym.QuoteAsset),
			PriceChangePercent24h: parseFloat(ticker.PriceChange24h, "UpdateAllSpotPairs: parsing PriceChange24h") * 100,
			BaseVolume24h:         parseFloat(ticker.BaseVolume24h, "UpdateAllSpotPairs: parsing BaseVolume24h"),
			QuoteVolume24h:        parseFloat(ticker.QuoteVolume24h, "UpdateAllSpotPairs: parsing QuoteVolume24h"),
			UpdatedAt:             time.Now(),
			CreatedAt:             time.Now(),
		}
		pairs = append(pairs, pair)
	}

	if len(pairs) == 0 {
		log.Println("No trading pairs found")
		return false
	}

	tx, err := db.Begin()
	if err != nil {
		log.Printf("Failed to begin transaction: %v", err)
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
		log.Printf("Failed to prepare statement: %v", err)
		return false
	}
	defer stmt.Close()

	args := make([]interface{}, 0, len(pairs)*13)
	for _, pair := range pairs {
		args = append(args, pair.PairKey, pair.Symbol, pair.Exchange, pair.Market, pair.Price, pair.BaseAsset, pair.QuoteAsset,
			pair.DisplayName, pair.PriceChangePercent24h, pair.BaseVolume24h, pair.QuoteVolume24h, pair.UpdatedAt, pair.CreatedAt)
	}

	_, err = stmt.Exec(args...)
	if err != nil {
		tx.Rollback()
		log.Printf("Failed to execute statement: %v", err)
		return false
	}

	if err := tx.Commit(); err != nil {
		log.Printf("Failed to commit transaction: %v", err)
		return false
	}

	return true
}

func UpdateAllFuturesPairs(db *sql.DB) bool {
	var wg sync.WaitGroup
	errChan := make(chan error, 2)

	var futuresData TickerResponseFutures
	var symbols SymbolsResponse

	wg.Add(2)
	go fetchJSON(tickerFuturesURL, &futuresData, &wg, errChan)
	go fetchJSON(symbolsFuturesURL, &symbols, &wg, errChan)

	wg.Wait()
	close(errChan)

	for err := range errChan {
		if err != nil {
			log.Printf("Bybit Error: %v", err)
			return false
		}
	}

	symbolMap := make(map[string]struct {
		Symbol     string
		BaseAsset  string
		QuoteAsset string
	})
	for _, sym := range symbols.Result.List {
		symbolMap[sym.Symbol] = struct {
			Symbol     string
			BaseAsset  string
			QuoteAsset string
		}{
			Symbol:     sym.Symbol,
			BaseAsset:  sym.BaseAsset,
			QuoteAsset: sym.QuoteAsset,
		}
	}

	var pairs []models.PairFutures
	for _, data := range futuresData.Result.List {
		symbolInfo, exists := symbolMap[data.Symbol]
		if !exists {
			// log.Printf("Bybit Warning: Symbol %s not found in symbolsFuturesURL", data.Symbol)
			continue
		}
		if data.FundingRate == "" {
			continue
		}
		pair := models.PairFutures{
			PairKey:               fmt.Sprintf("%s_Bybit_futures", data.Symbol),
			Symbol:                data.Symbol,
			Exchange:              "Bybit",
			Market:                "futures",
			MarkPrice:             parseFloat(data.LastPrice, "UpdateAllFuturesPairs: parsing LastPrice as MarkPrice"),
			IndexPrice:            parseFloat(data.LastPrice, "UpdateAllFuturesPairs: parsing LastPrice as IndexPrice"),
			BaseAsset:             symbolInfo.BaseAsset,
			QuoteAsset:            symbolInfo.QuoteAsset,
			DisplayName:           fmt.Sprintf("%s/%s", symbolInfo.BaseAsset, symbolInfo.QuoteAsset),
			FundingRatePercent:    parseFloat(data.FundingRate, "UpdateAllFuturesPairs: parsing FundingRate"),
			NextFundingTimestamp:  int(parseFloat(data.BaseVolume24h, "UpdateAllFuturesPairs: parsing BaseVolume24h as NextFundingTimestamp")),
			PriceChangePercent24h: parseFloat(data.PriceChange24h, "UpdateAllFuturesPairs: parsing PriceChange24h") * 100,
			BaseVolume24h:         parseFloat(data.BaseVolume24h, "UpdateAllFuturesPairs: parsing BaseVolume24h"),
			QuoteVolume24h:        parseFloat(data.QuoteVolume24h, "UpdateAllFuturesPairs: parsing QuoteVolume24h"),
			UpdatedAt:             time.Now(),
			CreatedAt:             time.Now(),
		}
		pairs = append(pairs, pair)
	}

	if len(pairs) == 0 {
		log.Printf("Bybit No futures pairs to update")
		return false
	}

	tx, err := db.Begin()
	if err != nil {
		log.Printf("Bybit Failed to begin transaction: %v", err)
		return false
	}

	placeholderStr := generateNumberedPlaceholders(len(pairs), 16)
	query := `
    INSERT INTO pairsfutures (pairkey, symbol, exchange, market, markprice, indexprice, baseasset, quoteasset, displayname, fundingRatePercent, nextfundingtimestamp, pricechangepercent24h, basevolume24h, quotevolume24h, updatedat, createdat)
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
		log.Printf("Bybit Failed to prepare statement: %v", err)
		return false
	}
	defer stmt.Close()

	args := make([]interface{}, 0, len(pairs)*16)
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
			pair.CreatedAt,
		)
	}

	_, err = stmt.Exec(args...)
	if err != nil {
		tx.Rollback()
		log.Printf("Bybit Failed to execute statement: %v", err)
		return false
	}

	if err := tx.Commit(); err != nil {
		log.Printf("Bybit Failed to commit transaction: %v", err)
		return false
	}

	return true
}
