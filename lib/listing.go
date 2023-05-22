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
	_, err = SetListing.Exec(l.Txid, l.Vout)
	return
}

// type PSBTListing struct {
// 	ListTxid ByteString `json:"ltxid"`
// 	ListVout uint32     `json:"lvout"`
// 	ListSeq  uint32     `json:"lseq"`
// 	Height   uint32     `json:"height"`
// 	Idx      uint32     `json:"idx"`
// 	Txid     ByteString `json:"txid"`
// 	Vout     uint32     `json:"vout"`
// 	Price    uint64     `json:"price"`
// 	Rawtx    []byte     `json:"rawtx"`
// 	Origin   Outpoint   `json:"origin"`
// }

// func (l *PSBTListing) Save() (err error) {
// 	_, err = InsListing.Exec(
// 		l.ListTxid,
// 		l.ListVout,
// 		l.ListSeq,
// 		l.Height,
// 		l.Idx,
// 		l.Txid,
// 		l.Vout,
// 		l.Price,
// 		l.Rawtx,
// 		l.Origin,
// 	)
// 	if err != nil {
// 		return
// 	}

// 	return
// }
