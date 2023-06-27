package lib

type Txo struct {
	Txid     ByteString `json:"txid"`
	Vout     uint32     `json:"vout"`
	Outpoint *Outpoint  `json:"outpoint"`
	// Height     *uint32       `json:"height"`
	// Idx        *uint64       `json:"idx"`
	Satoshis   uint64        `json:"satoshis,omitempty"`
	OutAcc     uint64        `json:"outacc,omitempty"`
	Scripthash ByteString    `json:"scripthash"`
	Lock       ByteString    `json:"lock"`
	Spend      ByteString    `json:"spend,omitempty"`
	Vin        uint32        `json:"vin"`
	InAcc      uint64        `json:"inacc,omitempty"`
	Origin     *Outpoint     `json:"origin,omitempty"`
	Listing    bool          `json:"listing,omitempty"`
	Bsv20      bool          `json:"bsv20,omitempty"`
	InOrd      *Txo          `json:"inOrd"`
	Parsed     *ParsedScript `json:"parsed,omitempty"`
}
