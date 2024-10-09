package lib

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

const ChaintipKey = "chaintip"
const BlocksKey = "blocks"
const TxStatusKey = "tx:stat"
const IngestKey = "ingest"
const OwnerSyncKey = "own:sync"
const OwnerAccountKey = "own:act"

func TxoKey(outpoint *Outpoint) string {
	return "txo:" + outpoint.String()
}

func SpendKey(outpoint *Outpoint) string {
	return "spd:" + outpoint.String()
}

func OwnerTxosKey(owner string) string {
	return "own:txo:" + owner
}

func AccountTxosKey(acct string) string {
	return "act:txo:" + acct
}

func ValidateKey(tag string) string {
	return "val:" + tag
}

func AccountKey(owner string) string {
	return "act:" + owner
}

func PubEventKey(tag string, event *Event) string {
	return fmt.Sprintf("evt:%s:%s:%s:%s", tag, event.Id, event.Value)
}

func PubOwnerKey(owner string) string {
	return "own:" + owner
}

func PubAccountKey(owner string) string {
	return "act:" + owner
}

func SpendValue(txid []byte, score float64) []byte {
	buf := new(bytes.Buffer)
	buf.Write(txid)
	binary.Write(buf, binary.BigEndian, score)
	return buf.Bytes()
}

func ParseSpendValue(val []byte) (txid []byte, score float64, err error) {
	buf := bytes.NewReader(val)
	txid = make([]byte, 32)
	if _, err := buf.Read(txid); err != nil {
		return nil, 0, err
	} else if err := binary.Read(buf, binary.BigEndian, &score); err != nil {
		return nil, 0, err
	} else {
		return txid, score, nil
	}
}
