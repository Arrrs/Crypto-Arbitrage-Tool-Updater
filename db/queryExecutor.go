package db

import (
	"database/sql"
	"fmt"
	// "io/ioutil"
	"log"
	"os"
)

// ExecuteSQL виконує переданий SQL-запит
func ExecuteSQL(db *sql.DB, query string) error {
	_, err := db.Exec(query)
	if err != nil {
		log.Printf("Error executing SQL query: %v", err)
		return err
	}
	// log.Println("SQL query executed successfully")
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
