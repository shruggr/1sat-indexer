package lib

import (
	"fmt"
)

const ChaintipKey = "chaintip"
const BlockHeightKey = "block:height"
const BlockHeadersKey = "block:headers"
const TxStatusKey = "tx:stat"
const IngestKey = "ingest"
const OwnerSyncKey = "own:sync"
const OwnerAccountKey = "own:act"
const TxosKey = "txos"

// const TxoDataKey = "txo:data"
const SpendsKey = "spends"

func OwnerTxosKey(owner string) string {
	return "own:txo:" + owner
}

func AccountTxosKey(account string) string {
	return "act:txo:" + account
}

func ValidateKey(tag string) string {
	return "val:" + tag
}

func AccountKey(account string) string {
	return "act:" + account
}

func PubEventKey(tag string, event *Event) string {
	return fmt.Sprintf("evt:%s:%s:%s", tag, event.Id, event.Value)
}

func PubOwnerKey(owner string) string {
	return "own:" + owner
}

func PubAccountKey(account string) string {
	return "act:" + account
}

func HeightScore(height uint32, idx uint64) float64 {
	return float64(uint64(height)*1000000000 + idx)
}
