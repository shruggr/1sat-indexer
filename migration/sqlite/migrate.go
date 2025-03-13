package main

import (
	"database/sql"
	"io/ioutil"
	"log"
	"os"

	"github.com/joho/godotenv"
	_ "github.com/mattn/go-sqlite3"
)

func main() {
	godotenv.Load("../../.env")
	dbPath := os.Getenv("SQLITE")
	migrationPath := "1_blockchain.up.sql"

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		db, err := sql.Open("sqlite3", dbPath)
		if err != nil {
			log.Fatal(err)
		}
		defer db.Close()

		migration, err := ioutil.ReadFile(migrationPath)
		if err != nil {
			log.Fatal(err)
		}

		if _, err := db.Exec(string(migration)); err != nil {
			log.Fatal(err)
		}

		log.Println("Database initialized successfully.")
	} else {
		log.Println("Database already exists.")
	}
}
