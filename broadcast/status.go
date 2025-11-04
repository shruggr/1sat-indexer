package broadcast

import "github.com/bsv-blockchain/go-sdk/transaction/broadcaster"

// IsAcceptedStatus returns true if the Arc status indicates successful broadcast
// This includes mempool acceptance, mined, and confirmed states
func IsAcceptedStatus(status broadcaster.ArcStatus) bool {
	switch status {
	case broadcaster.ACCEPTED_BY_NETWORK,
		broadcaster.SEEN_ON_NETWORK,
		broadcaster.MINED,
		broadcaster.CONFIRMED:
		return true
	default:
		return false
	}
}

// IsErrorStatus returns true if the Arc status indicates an error/rejection
func IsErrorStatus(status broadcaster.ArcStatus) bool {
	switch status {
	case broadcaster.REJECTED,
		broadcaster.DOUBLE_SPEND_ATTEMPTED,
		broadcaster.SEEN_IN_ORPHAN_MEMPOOL:
		return true
	default:
		return false
	}
}
