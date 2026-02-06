package api

import (
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// SetupRouter створює маршрути API
func SetupRouter(db *sql.DB) *gin.Engine {
	router := gin.Default()

	// Додаємо CORS middleware
	router.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
	}))

	router.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "Crypto Updater API is running!"})
	})

	router.GET("/api/health", func(c *gin.Context) {
		err := db.Ping()
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unhealthy", "db": "disconnected"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "healthy", "db": "connected"})
	})

	router.GET("/diffs", func(c *gin.Context) {
		// Отримуємо параметри запиту
		topRows := c.Query("topRows") // Якщо 0, то 500 за замовчуванням
		exchangesParam := c.DefaultQuery("exchanges", "")
		exchanges := strings.Split(exchangesParam, ",") // Масив бірж
		maxDiffPerc := c.Query("maxDiffPerc")
		minDiffPerc := c.Query("minDiffPerc")
		symbols := c.QueryArray("symbol") // Масив символів
		// coins := c.QueryArray("coins")    // Масив монет
		maxLifeTime := c.Query("maxLifeTime")
		minLifeTime := c.Query("minLifeTime")

		// fmt.Println("params - topRows:", topRows)
		// fmt.Println("params - exchanges:", exchanges)
		// fmt.Println("params - maxDiffPerc:", maxDiffPerc)
		// fmt.Println("params - minDiffPerc:", minDiffPerc)
		// fmt.Println("params - symbols:", symbols)
		// fmt.Println("params - maxLifeTime:", maxLifeTime)
		// fmt.Println("params - minLifeTime:", minLifeTime)

		// Формуємо динамічний SQL-запит
		query := "SELECT * FROM diffs WHERE 1=1"

		// Фільтрація за біржами (firstPairExchange та secondPairExchange)
		if len(exchanges) > 0 && exchanges[0] != "" {
			exchangeList := "'" + strings.Join(exchanges, "','") + "'"
			query += " AND firstPairExchange IN (" + exchangeList + ") "
			query += " AND secondPairExchange IN (" + exchangeList + ") "
		}

		// Фільтрація за відсотковою різницею
		if maxDiffPerc != "" && maxDiffPerc != "undefined" && maxDiffPerc != "0" {
			if _, err := strconv.ParseFloat(maxDiffPerc, 64); err == nil {
				query += " AND differencePercentage <= " + maxDiffPerc
			}
		}
		if minDiffPerc != "" && minDiffPerc != "undefined" && minDiffPerc != "0" {
			if _, err := strconv.ParseFloat(minDiffPerc, 64); err == nil {
				query += " AND differencePercentage >= " + minDiffPerc
			}
		}

		// Фільтрація за символами
		if len(symbols) > 0 && symbols[0] != "" {
			symbolList := "'" + strings.Join(symbols, "','") + "'"
			query += " AND symbol IN (" + symbolList + ")"
		}

		// Фільтрація за монетами (пошук по символу)
		// if len(coins) > 0 && coins[0] != "" {
		// 	coinConditions := []string{}
		// 	for _, coin := range coins {
		// 		coinConditions = append(coinConditions, "symbol LIKE '%"+coin+"%'")
		// 	}
		// 	query += " AND (" + strings.Join(coinConditions, " OR ") + ")"
		// }

		// Фільтрація за часом життя
		if maxLifeTime != "" && maxLifeTime != "undefined" {
			query += " AND timeElapsed <= INTERVAL '" + maxLifeTime + "'"
		}
		if minLifeTime != "" && minLifeTime != "undefined" {
			query += " AND timeElapsed >= INTERVAL '" + minLifeTime + "'"
		}

		query += " AND firstPairVolume <> 0"
		query += " AND secondPairVolume <> 0"
		query += " AND differencePercentage < 100000"

		// Обмеження кількості рядків
		if topRows == "" || topRows == "0" || topRows == "undefined" {
			query += " ORDER BY differencePercentage DESC LIMIT 500"
		} else if strings.ToLower(topRows) != "all" {
			if _, err := strconv.Atoi(topRows); err == nil {
				query += " ORDER BY differencePercentage DESC LIMIT " + topRows
			} else {
				query += " ORDER BY differencePercentage DESC LIMIT 500" // Якщо не число, використовуємо дефолтне значення
			}
		}

		// Виводимо фінальний SQL-запит у консоль
		fmt.Println("Final SQL Query:", query)

		// Виконуємо запит до бази
		rows, err := db.Query(query)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch data", "details": err.Error()})
			return
		}
		defer rows.Close()

		// Обробка результатів
		var results []map[string]interface{}
		cols, _ := rows.Columns()
		for rows.Next() {
			values := make([]interface{}, len(cols))
			valuePtrs := make([]interface{}, len(cols))
			for i := range values {
				valuePtrs[i] = &values[i]
			}

			rows.Scan(valuePtrs...)

			rowMap := make(map[string]interface{})
			for i, col := range cols {
				val := values[i]
				switch v := val.(type) {
				case []byte:
					strVal := string(v)
					if numVal, err := strconv.ParseFloat(strVal, 64); err == nil {
						rowMap[col] = numVal
					} else {
						rowMap[col] = strVal
					}
				default:
					rowMap[col] = val
				}
			}
			results = append(results, rowMap)
		}

		c.JSON(http.StatusOK, results)
	})

	router.GET("/diffsFutures", func(c *gin.Context) {
		// Отримуємо параметри запиту
		topRows := c.Query("topRows") // Якщо 0, то 500 за замовчуванням
		exchangesParam := c.DefaultQuery("exchanges", "")
		exchanges := strings.Split(exchangesParam, ",") // Масив бірж
		symbols := c.QueryArray("symbol")
		opposite := c.DefaultQuery("opposite", "false") // Отримуємо значення "opposite"
		coins := c.QueryArray("coins")                  // Масив монет

		// Формуємо динамічний SQL-запит
		query := "SELECT * FROM diffsfutures WHERE 1=1"

		// Фільтрація за біржами (firstPairExchange та secondPairExchange)
		if len(exchanges) > 0 && exchanges[0] != "" {
			exchangeList := "'" + strings.Join(exchanges, "','") + "'"
			query += " AND firstPairExchange IN (" + exchangeList + ") "
			query += " AND secondPairExchange IN (" + exchangeList + ") "
		}

		// Фільтрація за символами
		if len(symbols) > 0 && symbols[0] != "" {
			symbolList := "'" + strings.Join(symbols, "','") + "'"
			query += " AND symbol IN (" + symbolList + ")"
		}

		// Додаємо критерій пошуку, якщо opposite == true
		if strings.ToLower(opposite) == "true" {
			query += " AND isFundingRateOpposite = true"
		}

		// Фільтрація за монетами (пошук по символу)
		if len(coins) > 0 && coins[0] != "" {
			coinConditions := []string{}
			for _, coin := range coins {
				coinConditions = append(coinConditions, "(baseAsset = '"+coin+"' OR quoteAsset = '"+coin+"')")
			}
			query += " AND (" + strings.Join(coinConditions, " OR ") + ")"
		}

		query += " AND firstPairVolume <> 0"
		query += " AND secondPairVolume <> 0"

		// Обмеження кількості рядків
		if topRows == "" || topRows == "0" || topRows == "undefined" {
			query += " ORDER BY differenceFundingRatePercent DESC LIMIT 500"
		} else if strings.ToLower(topRows) != "all" {
			if _, err := strconv.Atoi(topRows); err == nil {
				query += " ORDER BY differenceFundingRatePercent DESC LIMIT " + topRows
			} else {
				query += " ORDER BY differenceFundingRatePercent DESC LIMIT 500" // Якщо не число, використовуємо дефолтне значення
			}
		}

		// Виводимо фінальний SQL-запит у консоль
		fmt.Println("Final SQL Query:", query)

		// Виконуємо запит до бази
		rows, err := db.Query(query)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch data", "details": err.Error()})
			return
		}
		defer rows.Close()

		// Обробка результатів
		var results []map[string]interface{}
		cols, _ := rows.Columns()
		for rows.Next() {
			values := make([]interface{}, len(cols))
			valuePtrs := make([]interface{}, len(cols))
			for i := range values {
				valuePtrs[i] = &values[i]
			}

			rows.Scan(valuePtrs...)

			rowMap := make(map[string]interface{})
			for i, col := range cols {
				val := values[i]
				switch v := val.(type) {
				case []byte:
					strVal := string(v)
					if numVal, err := strconv.ParseFloat(strVal, 64); err == nil {
						rowMap[col] = numVal
					} else {
						rowMap[col] = strVal
					}
				default:
					rowMap[col] = val
				}
			}
			results = append(results, rowMap)
		}

		c.JSON(http.StatusOK, results)
	})

	router.GET("/pairs", func(c *gin.Context) {
		// Виконуємо запит до бази для отримання унікальних символів
		symbolsQuery := "SELECT DISTINCT symbol FROM Pairs"
		symbolsRows, err := db.Query(symbolsQuery)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch symbols", "details": err.Error()})
			return
		}
		defer symbolsRows.Close()

		var symbols []string
		for symbolsRows.Next() {
			var symbol string
			if err := symbolsRows.Scan(&symbol); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to scan symbol", "details": err.Error()})
				return
			}
			symbols = append(symbols, symbol)
		}

		// Виконуємо запит до бази для отримання унікальних бірж
		exchangesQuery := "SELECT DISTINCT exchange FROM Pairs"
		exchangesRows, err := db.Query(exchangesQuery)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch exchanges", "details": err.Error()})
			return
		}
		defer exchangesRows.Close()

		var exchanges []string
		for exchangesRows.Next() {
			var exchange string
			if err := exchangesRows.Scan(&exchange); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to scan exchange", "details": err.Error()})
				return
			}
			exchanges = append(exchanges, exchange)
		}

		// Виконуємо запит до бази для отримання унікальних монет
		coinsQuery := "SELECT DISTINCT baseAsset, quoteAsset FROM Pairs"
		coinsRows, err := db.Query(coinsQuery)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch coins", "details": err.Error()})
			return
		}
		defer coinsRows.Close()

		var coins []string
		for coinsRows.Next() {
			var baseAsset, quoteAsset string
			if err := coinsRows.Scan(&baseAsset, &quoteAsset); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to scan coin", "details": err.Error()})
				return
			}
			coins = append(coins, baseAsset+"/"+quoteAsset)
		}

		// Повертаємо результати у форматі JSON
		c.JSON(http.StatusOK, gin.H{
			"symbols":   symbols,
			"exchanges": exchanges,
			"coins":     coins,
		})
	})

	router.GET("/pairsFutures", func(c *gin.Context) {
		// Виконуємо запит до бази для отримання унікальних символів
		symbolsQuery := "SELECT DISTINCT symbol FROM pairsfutures"
		symbolsRows, err := db.Query(symbolsQuery)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch symbols", "details": err.Error()})
			return
		}
		defer symbolsRows.Close()

		var symbols []string
		for symbolsRows.Next() {
			var symbol string
			if err := symbolsRows.Scan(&symbol); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to scan symbol", "details": err.Error()})
				return
			}
			symbols = append(symbols, symbol)
		}

		// Виконуємо запит до бази для отримання унікальних бірж
		exchangesQuery := "SELECT DISTINCT exchange FROM pairsfutures"
		exchangesRows, err := db.Query(exchangesQuery)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch exchanges", "details": err.Error()})
			return
		}
		defer exchangesRows.Close()

		var exchanges []string
		for exchangesRows.Next() {
			var exchange string
			if err := exchangesRows.Scan(&exchange); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to scan exchange", "details": err.Error()})
				return
			}
			exchanges = append(exchanges, exchange)
		}

		// Виконуємо запит до бази для отримання унікальних монет
		coinsQuery := `
			SELECT DISTINCT asset FROM (
				SELECT baseAsset AS asset FROM pairsfutures
				UNION
				SELECT quoteAsset AS asset FROM pairsfutures
			) AS combinedAssets
		`
		coinsRows, err := db.Query(coinsQuery)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch coins", "details": err.Error()})
			return
		}
		defer coinsRows.Close()

		var coins []string
		for coinsRows.Next() {
			var asset string
			if err := coinsRows.Scan(&asset); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to scan asset", "details": err.Error()})
				return
			}
			coins = append(coins, asset)
		}

		// Повертаємо результати у форматі JSON
		c.JSON(http.StatusOK, gin.H{
			"symbols":   symbols,
			"exchanges": exchanges,
			"coins":     coins,
		})
	})

	router.POST("/recreateTables", func(c *gin.Context) {
		fmt.Println("--- Post delete run")
		err := executeSQLFromFile(db, "db/queries/recreateTables.sql")
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to recreate tables", "details": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "Tables recreated successfully"})
	})

	return router
}

func executeSQLFromFile(db *sql.DB, filePath string) error {
	fmt.Println("--- Post delete executeSQLFromFile run")

	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read SQL file: %w", err)
	}
	fmt.Printf("--- Content SQL file length: %d\n", len(content))

	_, err = db.Exec(string(content))
	if err != nil {
		return fmt.Errorf("failed to execute SQL: %w", err)
	}

	return nil
}
