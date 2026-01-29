package db

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"
)

// ExecuteSQL виконує переданий SQL-запит з retry при deadlock
func ExecuteSQL(db *sql.DB, query string) error {
	maxRetries := 3
	for i := 0; i < maxRetries; i++ {
		_, err := db.Exec(query)
		if err == nil {
			return nil
		}

		// Check if it's a deadlock error
		if strings.Contains(err.Error(), "deadlock") {
			if i < maxRetries-1 {
				// Wait a bit and retry
				time.Sleep(time.Duration(100*(i+1)) * time.Millisecond)
				continue
			}
		}

		return err
	}
	return nil
}

// LoadSQLFromFile читає SQL-запит із файлу
func LoadSQLFromFile(filePath string) (string, error) {
	query, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("error reading SQL file %s: %v", filePath, err)
	}
	return string(query), nil
}
