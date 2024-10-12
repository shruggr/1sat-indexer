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
const OwnerAccountKey = "own:acct"
const TxosKey = "txos"
const ProgressKey = "progress"

// const TxoDataKey = "txo:data"
const SpendsKey = "spends"

func TxoDataKey(outpoint string) string {
	return "txo:data:" + outpoint
}
func OwnerTxosKey(owner string) string {
	return "own:txo:" + owner
}

func AccountTxosKey(account string) string {
	return "acct:txo:" + account
}

func PostProcessingKey(tag string) string {
	return "post:" + tag
}

func AccountKey(account string) string {
	return "acct:" + account
}

func TagKey(tag string) string {
	return "tag:" + tag
}

func EventKey(tag string, event *Event) string {
	return fmt.Sprintf("evt:%s:%s:%s", tag, event.Id, event.Value)
}

func PubEventKey(tag string, event *Event) string {
	return fmt.Sprintf("evt:%s:%s:%s", tag, event.Id, event.Value)
}

func PubOwnerKey(owner string) string {
	return "own:" + owner
}

func PubAccountKey(account string) string {
	return "acct:" + account
}

func HeightScore(height uint32, idx uint64) float64 {
	return float64(uint64(height)*1000000000 + idx)
}
