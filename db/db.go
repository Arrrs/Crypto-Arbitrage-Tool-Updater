package db

import (
	"database/sql"
	"log"

	_ "github.com/lib/pq"
)

// Connect creates a connection to the PostgreSQL database.
func Connect(databaseURL string) (*sql.DB, error) {
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return nil, err
	}

	// Verify the connection with a ping.
	if err := db.Ping(); err != nil {
		return nil, err
	}

	log.Printf("Database connected successfully ...")

	return db, nil
}
