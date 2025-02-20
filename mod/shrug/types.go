package shrug

import (
	"encoding/json"
	"errors"
	"math/big"

	"github.com/shruggr/1sat-indexer/v5/lib"
)

type ShrugStatus int

const (
	Invalid ShrugStatus = -1
	Pending ShrugStatus = 0
	Valid   ShrugStatus = 1
)

func (s ShrugStatus) String() string {
	switch s {
	case Invalid:
		return "invalid"
	case Valid:
		return "valid"
	default:
		return "pending"
	}
}

type Shrug struct {
	Id     *lib.Outpoint
	Amount *big.Int
	Status ShrugStatus
}

func ShrugFromBytes(data []byte) (*Shrug, error) {
	obj := &Shrug{}
	if err := json.Unmarshal(data, &obj); err != nil {
		return nil, err
	}
	return obj, nil
}

func (s *Shrug) MarshalJSON() ([]byte, error) {
	return json.Marshal(&struct {
		TokenId *lib.Outpoint `json:"tokenId"`
		Amount  string        `json:"amount"`
		Status  int           `json:"valid"`
	}{
		TokenId: s.Id,
		Amount:  s.Amount.String(),
		Status:  int(s.Status),
	})
}

func (s *Shrug) UnmarshalJSON(data []byte) error {
	var obj struct {
		TokenId *lib.Outpoint `json:"tokenId"`
		Amount  string        `json:"amount"`
		Status  int           `json:"valid"`
	}
	if err := json.Unmarshal(data, &obj); err != nil {
		return err
	}
	s.Id = obj.TokenId
	s.Status = ShrugStatus(obj.Status)
	s.Amount = new(big.Int)
	if _, ok := s.Amount.SetString(obj.Amount, 10); !ok {
		return errors.New("invalid-amount")
	}
	return nil
}
