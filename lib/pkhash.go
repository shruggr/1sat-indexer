package lib

import (
	"encoding/json"

	"github.com/libsv/go-bt/bscript"
)

type PKHash []byte

func (p *PKHash) Address() (string, error) {
	add, err := bscript.NewAddressFromPublicKeyHash(*p, true)
	if err != nil {
		return "", err
	}
	return add.AddressString, nil
}

// MarshalJSON serializes ByteArray to hex
func (p PKHash) MarshalJSON() ([]byte, error) {
	add, err := p.Address()
	if err != nil {
		return nil, err
	}
	return json.Marshal(add)
}

func (p *PKHash) FromAddress(a string) error {
	script, err := bscript.NewP2PKHFromAddress(a)
	if err != nil {
		return err
	}

	pkh := []byte(*script)[3:23]
	*p = pkh
	return nil
}

func (p *PKHash) UnmarshalJSON(data []byte) error {
	var add string
	err := json.Unmarshal(data, &add)
	if err != nil {
		return err
	}
	return p.FromAddress(add)
}
