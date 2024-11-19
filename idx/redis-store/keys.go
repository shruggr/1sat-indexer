package redisstore

const TxosKey = "txos"
const SpendsKey = "spends"

func TxoDataKey(outpoint string) string {
	return "txo:data:" + outpoint
}

func BalanceKey(key string) string {
	return "bal:" + key
}

const OwnerSyncKey = "own:sync"
const OwnerAccountKey = "own:acct"

func QueueKey(tag string) string {
	return "que:" + tag
}

func LogKey(tag string) string {
	return "log:" + tag
}
