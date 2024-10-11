package lib

type AddressTxn struct {
	Address string `json:"address"`
	Txid    string `json:"transaction_id"`
	Height  uint32 `json:"block_height"`
	BlockId string `json:"block_hash"`
	Idx     uint64 `json:"block_index"`
}
