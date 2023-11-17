package main

import (
	"github.com/shruggr/1sat-indexer/indexer"
	"github.com/shruggr/1sat-indexer/lib"
)

var settled = make(chan uint32, 1000)

func main() {
	go func() {
		for height := range settled {
			err := lib.SetOriginNum(height)
			if err != nil {
				panic(err)
			}
		}
	}()
	err := indexer.Exec(
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
	settled <- height - 6
	return nil
}
