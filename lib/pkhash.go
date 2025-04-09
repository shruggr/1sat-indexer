package lib

import (
	"encoding/json"

	"github.com/bsv-blockchain/go-sdk/script"
)

type PKHash []byte

func (p *PKHash) Address(network ...Network) string {
	mainnet := true
	if len(network) > 0 {
		mainnet = network[0] != Testnet
	}
	add, _ := script.NewAddressFromPublicKeyHash(*p, mainnet)
	return add.AddressString
}

// MarshalJSON serializes ByteArray to hex
func (p PKHash) MarshalJSON() ([]byte, error) {
	add := p.Address()
	return json.Marshal(add)
}

func (p *PKHash) FromAddress(a string) error {
	if add, err := script.NewAddressFromString(a); err != nil {
		return err
	} else {
		*p = PKHash(add.PublicKeyHash)
	}
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
