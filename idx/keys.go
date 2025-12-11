package idx

func OwnerKey(owner string) string {
	return "own:" + owner
}

// OwnerSpentKey returns the key for spent events for an owner address
func OwnerSpentKey(owner string) string {
	return "osp:" + owner
}

func BalanceKey(key string) string {
	return "bal:" + key
}

const OwnerSyncKey = "own:sync"

func QueueKey(tag string) string {
	return "que:" + tag
}

const IngestTag = "ingest"
const IngestQueueKey = "que:ingest"

func LogKey(tag string) string {
	return "log:" + tag
}
