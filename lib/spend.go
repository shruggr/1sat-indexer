package lib

type Spend struct {
	Txid  ByteString `json:"txid"`
	Vout  uint32     `json:"vout"`
	Spend ByteString `json:"spend"`
	Vin   uint32     `json:"vin"`
}

func (s *Spend) Save() (err error) {
	_, err = SetSpend.Exec(
		s.Txid,
		s.Vout,
		s.Spend,
		s.Vin,
	)

	return
}
