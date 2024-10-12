package bopen

import (
	"crypto/sha256"
	"database/sql/driver"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"strconv"

	"github.com/bitcoin-sv/go-sdk/script"
	"github.com/bitcoin-sv/go-sdk/transaction"
	"github.com/bitcoinschema/go-bitcoin"
	"github.com/shruggr/1sat-indexer/lib"
)

type Sigmas []*Sigma

func (s Sigmas) Value() (driver.Value, error) {
	return json.Marshal(s)
}

func (s *Sigmas) Scan(value interface{}) error {
	b, ok := value.([]byte)
	if !ok {
		return errors.New("type assertion to []byte failed")
	}
	return json.Unmarshal(b, &s)
}

type Sigma struct {
	Algorithm string `json:"algorithm"`
	Address   string `json:"address"`
	Signature []byte `json:"signature"`
	Vin       uint32 `json:"vin"`
	Valid     bool   `json:"valid"`
}

type SigmaIndexer struct {
	lib.BaseIndexer
}

func (i *SigmaIndexer) Tag() string {
	return "sigma"
}

func (i *SigmaIndexer) Parse(idxCtx *lib.IndexContext, vout uint32) (idxData *lib.IndexData) {
	txo := idxCtx.Txos[vout]
	if bopen, ok := txo.Data[BOPEN]; ok {
		if sigs, ok := bopen.Data.(BOpen)[i.Tag()].(*Sigmas); ok {
			idxData = &lib.IndexData{
				Data: sigs,
			}
			for _, sig := range *sigs {
				idxData.Events = append(idxData.Events, &lib.Event{
					Id:    "address",
					Value: sig.Address,
				})
			}
		}
	}
	return
}

func ParseSigma(tx *transaction.Transaction, vout uint32, idx *int) (sigma *Sigma) {
	scr := *tx.Outputs[vout].LockingScript
	startIdx := *idx
	sigma = &Sigma{}
	for i := 0; i < 4; i++ {
		prevIdx := *idx
		op, err := scr.ReadOp(idx)
		if err != nil || op.Op == script.OpRETURN || (op.Op == 1 && op.Data[0] == '|') {
			*idx = prevIdx
			break
		}

		switch i {
		case 0:
			sigma.Algorithm = string(op.Data)
		case 1:
			sigma.Address = string(op.Data)
		case 2:
			sigma.Signature = op.Data
		case 3:
			vin, err := strconv.ParseInt(string(op.Data), 10, 32)
			if err == nil {
				sigma.Vin = uint32(vin)
			}
		}
	}

	outpoint := tx.Inputs[sigma.Vin].SourceTXID.CloneBytes()
	outpoint = binary.LittleEndian.AppendUint32(outpoint, tx.Inputs[sigma.Vin].SourceTxOutIndex)
	inputHash := sha256.Sum256(outpoint)
	var scriptBuf []byte
	if scr[startIdx-1] == script.OpRETURN {
		scriptBuf = scr[:startIdx-1]
	} else if scr[startIdx-1] == '|' {
		scriptBuf = scr[:startIdx-2]
	} else {
		return nil
	}
	outputHash := sha256.Sum256(scriptBuf)
	msgHash := sha256.Sum256(append(inputHash[:], outputHash[:]...))
	if err := bitcoin.VerifyMessage(sigma.Address,
		base64.StdEncoding.EncodeToString(sigma.Signature),
		string(msgHash[:]),
	); err != nil {
		sigma.Valid = false
	} else {
		sigma.Valid = true
	}
	return sigma
}
