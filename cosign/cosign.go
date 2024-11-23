package cosign

import (
	"encoding/hex"
	"encoding/json"

	"github.com/bitcoin-sv/go-sdk/script"
	"github.com/shruggr/1sat-indexer/v5/evt"
	"github.com/shruggr/1sat-indexer/v5/idx"
)

const COSIGN_TAG = "cosign"

type Cosign struct {
	Address  string `json:"address"`
	Cosigner string `json:"cosigner"`
}

type CosignIndexer struct {
	idx.BaseIndexer
}

func (i *CosignIndexer) Tag() string {
	return COSIGN_TAG
}

func (i *CosignIndexer) FromBytes(data []byte) (any, error) {
	return CosignFromBytes(data)
}

func CosignFromBytes(data []byte) (*Cosign, error) {
	obj := &Cosign{}
	if err := json.Unmarshal(data, &obj); err != nil {
		return nil, err
	}
	return obj, nil
}

func (i *CosignIndexer) Parse(idxCtx *idx.IndexContext, vout uint32) *idx.IndexData {
	txo := idxCtx.Txos[vout]
	lockingScript := idxCtx.Tx.Outputs[vout].LockingScript
	chunks, _ := lockingScript.Chunks()

	cosign := parseScript(chunks, 0)
	if cosign == nil {
		for i, chunk := range chunks {
			if chunk.Op == script.OpCODESEPARATOR {
				if cosign = parseScript(chunks, i+1); cosign != nil {
					break
				}
			}
		}
	}
	if cosign != nil {
		txo.AddOwner(cosign.Address)
		return &idx.IndexData{
			Data: cosign,
			Events: []*evt.Event{
				{
					Id:    "own",
					Value: cosign.Address,
				},
				{
					Id:    "cosigner",
					Value: cosign.Cosigner,
				},
			},
		}
	}
	return nil
}

func parseScript(chunks []*script.ScriptChunk, offset int) (cosign *Cosign) {
	if len(chunks) >= 7+offset &&
		chunks[0+offset].Op == script.OpDUP &&
		chunks[1+offset].Op == script.OpHASH160 &&
		len(chunks[2+offset].Data) == 20 &&
		chunks[3+offset].Op == script.OpEQUALVERIFY &&
		chunks[4+offset].Op == script.OpCHECKSIGVERIFY &&
		len(chunks[5+offset].Data) == 33 &&
		chunks[6+offset].Op == script.OpCHECKSIG {

		cosign = &Cosign{
			Cosigner: hex.EncodeToString(chunks[5+offset].Data),
		}
		if add, err := script.NewAddressFromPublicKeyHash(chunks[2+offset].Data, true); err == nil {
			cosign.Address = add.AddressString
		}
	}
	return
}
