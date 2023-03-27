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
	txid, _ := hex.DecodeString("a72cc94b8f6276f73e617b3954087bdad11982bfd913e3061975e5300d773681")
	tx, err := lib.LoadTx(txid)
	if err != nil {
		log.Panic(err)
	}

	result, err := lib.IndexInscriptionTxos(tx, 0, 0)
	if err != nil {
		log.Panic(err)
	}
	out, err := json.Marshal(result)
	if err != nil {
		log.Panic(err)
	}
	fmt.Println(string(out))
}
