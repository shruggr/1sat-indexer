package lib

import (
	"encoding/json"

	"github.com/GorillaPool/go-junglebus/models"
)

type BlockHeader models.BlockHeader

func (b BlockHeader) MarshalBinary() (data []byte, err error) {
	return json.Marshal(b)
}

func (b *BlockHeader) UnmarshalBinary(data []byte) (err error) {
	return json.Unmarshal(data, b)
}
