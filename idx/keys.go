package idx

func AccountKey(account string) string {
	return "acct:" + account
}

func OwnerKey(owner string) string {
	return "own:" + owner
}

func OwnerTxosKey(owner string) string {
	return "own:" + owner + ":txo"
}

func AccountTxosKey(account string) string {
	return "acct:" + account + ":txo"
}

func LockKey(outpoint string) string {
	return "lock:" + outpoint
}
