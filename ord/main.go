package main

import (
	"github.com/shruggr/1sat-indexer/junglebus"
	"github.com/shruggr/1sat-indexer/lib"
)

func main() {
	err := junglebus.Exec(
		true,
		true,
		handleTx,
		handleBlock,
	)
	if err != nil {
		panic(err)
	}
}

func handleTx(tx *lib.IndexContext) error {
	return nil
}

func handleBlock(height uint32) error {
	return nil
}
