package lib

type OrdLockListing struct {
	Txid      ByteString `json:"txid"`
	Vout      uint32     `json:"vout"`
	Height    uint32     `json:"height"`
	Idx       uint64     `json:"idx"`
	Price     uint64     `json:"price"`
	PayOutput ByteString `json:"pay_output"`
	Origin    *Outpoint  `json:"origin"`
	Ordinal   uint64     `json:"ordinal"`
	Outpoint  *Outpoint  `json:"outpoint,omitempty"`
	Lock      ByteString `json:"lock"`
	Bsv20     bool       `json:"bsv20"`
}

func (l *OrdLockListing) Save() (err error) {
	_, err = InsListing.Exec(
		l.Txid,
		l.Vout,
		l.Height,
		l.Idx,
		l.Price,
		l.PayOutput,
		l.Origin,
	)

	if err != nil {
		return
	}
	return
}
