package lib

import (
	"fmt"
)

const BlockHeightKey = "blk:height"
const BlockHeadersKey = "blk:headers"
const ChaintipKey = "blk:tip"
const IngestQueueKey = "que:ingest"
const PendingQueueKey = "que:pending"
const TxLogKey = "log:tx"
const OwnerSyncKey = "own:sync"
const OwnerAccountKey = "own:acct"
const TxosKey = "txos"

func TxKey(txid string) string {
	return "tx:" + txid
}

func ProofKey(txid string) string {
	return "prf:" + txid
}

func ProgressQueueKey(tag string) string {
	return "progress:" + tag
}

func QueueKey(tag string) string {
	return "que:" + tag
}

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
