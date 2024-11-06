package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
	"github.com/shruggr/1sat-indexer/blk"
)

const PAGE_SIZE = 10000

var JUNGLEBUS string
var ctx = context.Background()

func init() {
	wd, _ := os.Getwd()
	log.Println("CWD:", wd)
	godotenv.Load(fmt.Sprintf(`%s/../../.env`, wd))
}

func main() {
	blk.Sync(ctx)
}
