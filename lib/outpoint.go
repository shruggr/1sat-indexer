package lib

import (
	"encoding/json"
	"fmt"

	"github.com/bsv-blockchain/go-sdk/chainhash"
	"github.com/bsv-blockchain/go-sdk/transaction"
)

// Outpoint wraps transaction.Outpoint with underscore-format JSON marshalling
type Outpoint struct {
	transaction.Outpoint
}

// NewOutpoint creates a new Outpoint from a transaction.Outpoint
func NewOutpoint(op transaction.Outpoint) *Outpoint {
	return &Outpoint{Outpoint: op}
}

// NewOutpointFromHash creates an Outpoint from a chainhash and vout
func NewOutpointFromHash(txid *chainhash.Hash, vout uint32) *Outpoint {
	return &Outpoint{Outpoint: transaction.Outpoint{
		Txid:  *txid,
		Index: vout,
	}}
}

// NewOutpointFromBytes creates an Outpoint from 36 bytes (32 txid + 4 vout big-endian)
func NewOutpointFromBytes(b []byte) *Outpoint {
	if len(b) != 36 {
		return nil
	}
	op := transaction.NewOutpointFromBytes([36]byte(b))
	return &Outpoint{Outpoint: *op}
}

// NewOutpointFromString parses an outpoint string (supports both . and _ separators)
func NewOutpointFromString(s string) (*Outpoint, error) {
	op, err := transaction.OutpointFromString(s)
	if err != nil {
		return nil, err
	}
	return &Outpoint{Outpoint: *op}, nil
}

// String returns the outpoint in underscore format (txid_vout)
func (o *Outpoint) String() string {
	return fmt.Sprintf("%s_%d", o.Txid.String(), o.Index)
}

// Bytes returns the outpoint as 36 bytes (big-endian format)
func (o *Outpoint) Bytes() []byte {
	return o.Outpoint.Bytes()
}

// MarshalJSON serializes to underscore format
func (o Outpoint) MarshalJSON() ([]byte, error) {
	return json.Marshal(o.String())
}

// UnmarshalJSON deserializes from string (supports both . and _ separators)
func (o *Outpoint) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	op, err := NewOutpointFromString(s)
	if err != nil {
		return err
	}
	*o = *op
	return nil
}
