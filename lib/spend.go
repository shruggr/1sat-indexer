package lib

import (
	"bytes"
	"encoding/binary"
)

type Spend struct {
	Txid     ByteString `json:"txid"`
	Vin      uint32     `json:"vin"`
	InTxid   ByteString `json:"inTxid"`
	InVout   uint32     `json:"inVout"`
	Satoshis uint64     `json:"satoshis"`
	AccSats  uint64     `json:"accSats"`
}

func (s *Spend) Save() (err error) {
	_, err = SetSpend.Exec(
		s.InTxid,
		s.InVout,
		s.Txid,
		s.Vin,
	)

	return
}

func (s *Spend) KVKey() []byte {
	buf := bytes.NewBuffer([]byte{'t'})
	binary.Write(buf, binary.BigEndian, s.Txid)
	binary.Write(buf, binary.BigEndian, 'i')
	binary.Write(buf, binary.BigEndian, s.AccSats)
	return buf.Bytes()
}

func (s *Spend) KVValue() []byte {
	buf := bytes.NewBuffer(nil)
	binary.Write(buf, binary.BigEndian, s.Vin)
	binary.Write(buf, binary.BigEndian, s.InTxid)
	binary.Write(buf, binary.BigEndian, s.InVout)
	binary.Write(buf, binary.BigEndian, s.Satoshis)
	return buf.Bytes()
}
