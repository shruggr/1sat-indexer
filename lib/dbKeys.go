package lib

import (
	"fmt"
)

const BlockHeightKey = "blk:height"
const BlockHeadersKey = "blk:headers"
const ChaintipKey = "blk:tip"

const TxStatusKey = "tx:stat"
const OwnerSyncKey = "own:sync"
const OwnerAccountKey = "own:acct"
const TxosKey = "txos"

func ProgressQueueKey(tag string) string {
	return "progress:" + tag
}

func IngestQueueKey(tag string) string {
	return "ingest:" + tag
}

func DepQueueKey(txid string) string {
	return "dep:" + txid
}

func IngestLogKey(tag string) string {
	return "ing:log:" + tag
}

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
