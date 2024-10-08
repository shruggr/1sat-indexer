package bopen

import (
	"github.com/bitcoin-sv/go-sdk/transaction"
)

var MAP = "1PuQa7K62MiKCtssSLKy1kh56WWU7MtUR5"
var B = "19HxigV4QyBv3tHpQVcUEQyq1pzZVdoAut"

func ParseBitcom(tx *transaction.Transaction, vout uint32, idx *int) (value interface{}, err error) {
	scr := *tx.Outputs[vout].LockingScript

	op, err := scr.ReadOp(idx)
	if err != nil {
		return
	}
	switch string(op.Data) {
	case MAP:
		value = ParseMAP(&scr, idx)

	case B:
		value = ParseB(&scr, idx)
	case "SIGMA":

		value = ParseSigma(tx, vout, idx)
	default:
		*idx--
	}
	return value, nil
}
