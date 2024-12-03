package redisstore

const TxosKey = "txos"
const SpendsKey = "spends"

func TxoDataKey(outpoint string) string {
	return "txo:data:" + outpoint
}
