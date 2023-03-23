package main

import (
	"database/sql"
	"encoding/hex"
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
	"github.com/libsv/go-bt/v2/bscript"
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

	b, _ := hex.DecodeString("2be2cf7cff0b954cdd919f3bb22155cdb128005b7e639918e26cd61a5672e976")
	script := bscript.NewFromBytes(b)
	pkh, err := script.PublicKeyHash()
	if err != nil {
		log.Panic(err)
	}
	add, err := bscript.NewAddressFromPublicKeyHash(pkh, true)
	if err != nil {
		log.Panic(err)
	}
	fmt.Println(add.AddressString)
}
