package lib

import (
	"encoding/json"
	"time"

	"github.com/bitcoin-sv/go-sdk/transaction"
	"github.com/vmihailenco/msgpack/v5"
)

type Event struct {
	Id    string `json:"id"`
	Value string `json:"value"`
}

type IndexData struct {
	Data     any         `json:"-"`
	Events   []*Event    `json:"events"`
	FullText string      `json:"text"`
	Deps     []*Outpoint `json:"deps"`
	Validate bool        `json:"validate"`
}

func (id IndexData) MarshalMsgpack() ([]byte, error) {
	data := make(map[string]any)
	if j, err := json.Marshal(id.Data); err != nil {
		return nil, err
	} else if err = json.Unmarshal(j, &data); err != nil {
		return nil, err
	} else {
		return msgpack.Marshal(data)
	}
}

func (id *IndexData) UnmarshalMsgpack(data []byte) error {
	return msgpack.Unmarshal(data, &id.Data)
}

func NewIndexContext(tx *transaction.Transaction, indexers []Indexer) *IndexContext {
	idxCtx := &IndexContext{
		Id:       uint64(time.Now().UnixNano()),
		Tx:       tx,
		Txid:     tx.TxID(),
		Indexers: indexers,
	}

	if tx.MerklePath != nil {
		idxCtx.Height = tx.MerklePath.BlockHeight
		for _, path := range tx.MerklePath.Path[0] {
			if idxCtx.Txid.IsEqual(path.Hash) {
				idxCtx.Idx = path.Offset
				break
			}
		}
	} else {
		idxCtx.Height = uint32(time.Now().Unix())
	}
	return idxCtx
}
