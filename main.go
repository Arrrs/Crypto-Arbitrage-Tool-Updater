package main

import (
	"log"
	"os"
	"sync"
	"time"

	"Updater/api"
	"Updater/config"
	"Updater/db"
	backpack "Updater/exchanges/backpack"
	binance "Updater/exchanges/binance"
	bitget "Updater/exchanges/bitget"
	bybit "Updater/exchanges/bybit"
	gate "Updater/exchanges/gate"
	huobi "Updater/exchanges/huobi"
	kraken "Updater/exchanges/kraken"
	kuCoin "Updater/exchanges/kuCoin"
	mexc "Updater/exchanges/mexc"
	okx "Updater/exchanges/okx"
	whiteBIT "Updater/exchanges/whiteBIT"

	"github.com/go-co-op/gocron/v2"
)

func main() {
	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Error loading configuration: %v", err)
	}

	// Connect to PostgreSQL database
	dbConn, err := db.Connect(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Database connection error: %v", err)
	}
	defer dbConn.Close()

	// Create scheduler
	s, err := gocron.NewScheduler()
	if err != nil {
		log.Fatalf("Error creating scheduler: %v", err)
	}

	// List of exchanges to update every 5 seconds

	exchanges := map[string]func() bool{
		"Backpack": func() bool { return backpack.UpdateAllSpotPairs(dbConn) },
		"Binance":  func() bool { return binance.UpdateAllSpotPairs(dbConn) },
		"Bitget":   func() bool { return bitget.UpdateAllSpotPairs(dbConn) },
		"Bybit":    func() bool { return bybit.UpdateAllSpotPairs(dbConn) },
		"Gate":     func() bool { return gate.UpdateAllSpotPairs(dbConn) },
		"Huobi":    func() bool { return huobi.UpdateAllSpotPairs(dbConn) },
		"Kraken":   func() bool { return kraken.UpdateAllSpotPairs(dbConn) },
		"KuCoin":   func() bool { return kuCoin.UpdateAllSpotPairs(dbConn) },
		"MEXC":     func() bool { return mexc.UpdateAllSpotPairs(dbConn) },
		"OKX":      func() bool { return okx.UpdateAllSpotPairs(dbConn) },
		"WhiteBIT": func() bool { return whiteBIT.UpdateAllSpotPairs(dbConn) },
	}
	networks := map[string]func() bool{
		"Binance": func() bool {
			return binance.UpdateAllNetworks(dbConn, os.Getenv("API_KEY_BINANCE"), os.Getenv("API_SECRET_BINANCE"))
		},
		"Bitget":   func() bool { return bitget.UpdateAllNetworks(dbConn) },
		"Huobi":    func() bool { return huobi.UpdateAllNetworks(dbConn) },
		"WhiteBIT": func() bool { return whiteBIT.UpdateAllNetworks(dbConn) },
	}
	futures := map[string]func() bool{
		"Backpack": func() bool { return backpack.UpdateAllFuturesPairs(dbConn) },
		"Binance":  func() bool { return binance.UpdateAllFuturesPairs(dbConn) },
		"Bybit":    func() bool { return bybit.UpdateAllFuturesPairs(dbConn) },
		"MEXC":     func() bool { return mexc.UpdateAllFuturesPairs(dbConn) },
	}

	for name, updateFunc := range exchanges {
		_, err := s.NewJob(
			gocron.DurationJob(20*time.Second),
			gocron.NewTask(func(exchange string, fn func() bool) {
				if fn() {
					// log.Printf("%s spot pairs updated.", exchange)
				} else {
					log.Printf("%s error updating spot pairs.", exchange)
				}
			}, name, updateFunc),
		)
		if err != nil {
			log.Fatalf("Error scheduling %s job: %v", name, err)
		}
	}
	for name, updateFunc := range networks {
		_, err := s.NewJob(
			gocron.DurationJob(150*time.Second),
			gocron.NewTask(func(exchange string, fn func() bool) {
				if fn() {
					// log.Printf("%s networks updated.", exchange)
				} else {
					log.Printf("%s error updating networks.", exchange)
				}
			}, name, updateFunc),
		)
		if err != nil {
			log.Fatalf("Error scheduling %s network job: %v", name, err)
		}
	}

	for name, updateFunc := range futures {
		_, err := s.NewJob(
			gocron.DurationJob(10*time.Second),
			gocron.NewTask(func(exchange string, fn func() bool) {
				if fn() {
					// log.Printf("%s spot pairs updated.", exchange)
				} else {
					log.Printf("%s error updating futures pairs.", exchange)
				}
			}, name, updateFunc),
		)
		if err != nil {
			log.Fatalf("Error scheduling %s job: %v", name, err)
		}
	}

	// Mutex to prevent diff jobs from running simultaneously (avoids deadlocks)
	var diffMutex sync.Mutex

	updateDiffsSqlJob, err := s.NewJob(
		gocron.DurationJob(10*time.Second),
		gocron.NewTask(
			func() {
				diffMutex.Lock()
				defer diffMutex.Unlock()

				query, err := db.LoadSQLFromFile("db/queries/updateDiffs.sql")
				if err != nil {
					log.Println("Error loading SQL file:", err)
					return
				}

				err = db.ExecuteSQL(dbConn, query)
				if err != nil {
					log.Println("Error executing SQL job (updateDiffs):", err)
				}
			},
		),
	)
	if err != nil {
		log.Fatalf("Error scheduling SQL job: %v", err)
	}
	log.Println("SQL job created (updateDiffs) with ID:", updateDiffsSqlJob.ID())

	updateDiffsFuturesSqlJob, err := s.NewJob(
		gocron.DurationJob(10*time.Second),
		gocron.NewTask(
			func() {
				diffMutex.Lock()
				defer diffMutex.Unlock()

				query, err := db.LoadSQLFromFile("db/queries/updateDiffsFutures.sql")
				if err != nil {
					log.Println("Error loading SQL file:", err)
					return
				}

				err = db.ExecuteSQL(dbConn, query)
				if err != nil {
					log.Println("Error executing SQL job (updateDiffsFutures):", err)
				}
			},
		),
	)
	if err != nil {
		log.Fatalf("Error scheduling SQL job: %v", err)
	}
	log.Println("SQL job created (updateDiffsFutures) with ID:", updateDiffsFuturesSqlJob.ID())

	// Start scheduler
	s.Start()

	// Start API server in a separate goroutine
	go func() {
		router := api.SetupRouter(dbConn)
		log.Printf("Starting API server on %s", cfg.APIPort)
		if err := router.Run(cfg.APIPort); err != nil {
			log.Fatalf("API server error: %v", err)
		}
	}()

	// Block indefinitely
	select {}
}
