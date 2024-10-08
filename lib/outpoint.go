package lib

import (
	"database/sql/driver"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/bitcoin-sv/go-sdk/util"
)

type Outpoint []byte

func NewOutpoint(txid []byte, vout uint32) *Outpoint {
	o := Outpoint(binary.LittleEndian.AppendUint32(util.ReverseBytes(txid), vout))
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
	origin := Outpoint(binary.LittleEndian.AppendUint32(util.ReverseBytes(txid), uint32(vout)))
	o = &origin
	return
}

func (o *Outpoint) String() string {
	return fmt.Sprintf("%x_%d", util.ReverseBytes((*o)[:32]), binary.LittleEndian.Uint32((*o)[32:]))
}

func (o *Outpoint) Txid() []byte {
	return util.ReverseBytes((*o)[:32])
}

func (o *Outpoint) TxidHex() string {
	return hex.EncodeToString(o.Txid())
}

func (o *Outpoint) Vout() uint32 {
	return binary.LittleEndian.Uint32((*o)[32:])
}

func (o Outpoint) MarshalJSON() (bytes []byte, err error) {
	if len(o) != 36 {
		return []byte("null"), nil
	}
	return json.Marshal(o.String())
}

// UnmarshalJSON deserializes Origin to string
func (o *Outpoint) UnmarshalJSON(data []byte) error {
	var x string
	err := json.Unmarshal(data, &x)
	if err != nil {
		return err
	} else if op, err := NewOutpointFromString(x); err != nil {
		return err
	} else {
		*o = *op
		return nil
	}
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
