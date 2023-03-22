package main

import (
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
	"github.com/shruggr/1sat-indexer/lib"
)

func main() {
	godotenv.Load("../.env")

	db, err := sql.Open("postgres", os.Getenv("POSTGRES"))
	if err != nil {
		log.Panic(err)
	}

	err = lib.Initialize(db)
	if err != nil {
		log.Panic(err)
	}

	lock, err := hex.DecodeString("9124b13c9c1d5320d7238d9ee9f41423e1775f736d349b2897982c251cb9623e")
	if err != nil {
		log.Panic(err)
	}
	fmt.Printf("Lock: %x\n", lock)
	utxos, err := lib.LoadUtxos(lock)
	if err != nil {
		log.Panic(err)
	}
	fmt.Printf("UTXOs: %v\n", utxos)
	str, err := json.Marshal(utxos)
	if err != nil {
		log.Panic(err)
	}
	fmt.Println(string(str))
}
