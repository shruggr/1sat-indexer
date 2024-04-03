package lib

import (
	"database/sql/driver"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/libsv/go-bt/v2"
)

type Outpoint []byte

func NewOutpoint(txid []byte, vout uint32) *Outpoint {
	o := Outpoint(binary.BigEndian.AppendUint32(txid, vout))
	return &o
}

func NewOutpointFromString(s string) (o *Outpoint, err error) {
	if len(s) < 66 {
		return nil, fmt.Errorf("invalid-string")
	}
	txid, err := hex.DecodeString(s[:64])
	if err != nil {
		return
	}
	vout, err := strconv.ParseUint(s[65:], 10, 32)
	if err != nil {
		return
	}
	origin := Outpoint(binary.BigEndian.AppendUint32(txid, uint32(vout)))
	o = &origin
	return
}

func NewOutpointFromTxOutpoint(p []byte) (o *Outpoint, err error) {
	if len(p) < 32 || len(p) > 36 {
		return nil, errors.New("invalid pointer")
	}
	b := make([]byte, 36)
	copy(b[:32], bt.ReverseBytes(p[:32]))
	if len(p) > 32 {
		copy(b[32:], bt.ReverseBytes(p[32:]))
	}
	// b = append(b, bt.ReverseBytes(p[32:])...)
	origin := Outpoint(b)
	o = &origin
	return
}

func (o *Outpoint) String() string {
	return fmt.Sprintf("%x_%d", (*o)[:32], binary.BigEndian.Uint32((*o)[32:]))
}

func (o *Outpoint) Txid() []byte {
	return (*o)[:32]
}

func (o *Outpoint) Vout() uint32 {
	return binary.BigEndian.Uint32((*o)[32:])
}

func (o Outpoint) MarshalJSON() (bytes []byte, err error) {
	if len(o) != 36 {
		return []byte("null"), nil
	}
	return json.Marshal(fmt.Sprintf("%x_%d", o[:32], binary.BigEndian.Uint32(o[32:])))
}

// UnmarshalJSON deserializes Origin to string
func (o *Outpoint) UnmarshalJSON(data []byte) error {
	var x string
	err := json.Unmarshal(data, &x)
	if err == nil {
		txid, err := hex.DecodeString(x[:64])
		if err != nil {
			return err
		}
		vout, err := strconv.ParseUint(x[65:], 10, 32)
		if err != nil {
			return err
		}

		*o = Outpoint(binary.BigEndian.AppendUint32(txid, uint32(vout)))
	}

	return err
}

func (o Outpoint) Value() (driver.Value, error) {
	return []byte(o), nil
}

func (o *Outpoint) Scan(value interface{}) error {
	b, ok := value.([]byte)
	if !ok {
		return errors.New("type assertion to []byte failed")
	}
	*o = b
	return nil
}
