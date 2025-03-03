package idx

func AccountKey(account string) string {
	return "acct:" + account
}

func OwnerKey(owner string) string {
	return "own:" + owner
}

func LockKey(outpoint string) string {
	return "lock:" + outpoint
}

func BalanceKey(key string) string {
	return "bal:" + key
}

const OwnerSyncKey = "own:sync"
const OwnerAccountKey = "own:acct"

func QueueKey(tag string) string {
	return "que:" + tag
}

const IngestTag = "ingest"
const IngestQueueKey = "que:ingest"

func LogKey(tag string) string {
	return "log:" + tag
}
