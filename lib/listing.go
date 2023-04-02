package lib

type Listing struct {
	ListTxid ByteString `json:"ltxid"`
	ListVout uint32     `json:"lvout"`
	ListSeq  uint32     `json:"lseq"`
	Height   uint32     `json:"height"`
	Idx      uint32     `json:"idx"`
	Txid     ByteString `json:"txid"`
	Vout     uint32     `json:"vout"`
	Price    uint64     `json:"price"`
	Rawtx    []byte     `json:"rawtx"`
	Origin   Outpoint   `json:"origin"`
}

func (l *Listing) Save() (err error) {
	_, err = InsListing.Exec(
		l.ListTxid,
		l.ListVout,
		l.ListSeq,
		l.Height,
		l.Idx,
		l.Txid,
		l.Vout,
		l.Price,
		l.Rawtx,
		l.Origin,
	)
	if err != nil {
		return
	}

	return
}
