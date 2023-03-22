package main

import (
	"database/sql"
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

	o, err := lib.NewOriginFromString("13899501db55c2c0d9a79b6fe0a84eac9d68a8e1b9971b05bfab8511608bd009_0")
	if err != nil {
		log.Panic(err)
	}
	fmt.Printf("Origin: %s\n", o.String())

	// lock, err := hex.DecodeString("dab3a9eecb41663021b01755fa924332a922e026b3669823b40e05e8689a7005")
	// if err != nil {
	// 	log.Panic(err)
	// }
	// fmt.Printf("Lock: %x\n", lock)
	// utxos, err := lib.LoadUtxos(lock)
	// if err != nil {
	// 	log.Panic(err)
	// }
	// fmt.Printf("UTXOs: %v\n", utxos)
	// for _, utxo := range utxos {
	// 	// o, err := lib.NewOriginFromString(utxo.Origin)
	// 	// if err != nil {
	// 	// 	log.Panic(err)
	// 	// }
	// 	fmt.Printf("Origin: %s\n", utxo.Origin.String())
	// }
	// str, err := json.Marshal(utxos)
	// if err != nil {
	// 	log.Panic(err)
	// }
	// fmt.Println(string(str))
}
