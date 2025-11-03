package broadcast

import "github.com/bsv-blockchain/go-sdk/transaction/broadcaster"

// IsFinalStatus returns true if the ARC status is considered final (terminal state)
func IsFinalStatus(status broadcaster.ArcStatus) bool {
	switch status {
	case broadcaster.ACCEPTED_BY_NETWORK,
		broadcaster.SEEN_ON_NETWORK,
		broadcaster.REJECTED:
		return true
	default:
		return false
	}
}

// IsErrorStatus returns true if the ARC status indicates an error/rejection
func IsErrorStatus(status broadcaster.ArcStatus) bool {
	switch status {
	case broadcaster.REJECTED:
		return true
	default:
		return false
	}
}
