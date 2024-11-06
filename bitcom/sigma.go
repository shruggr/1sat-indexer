package bitcom

import (
	"crypto/sha256"
	"database/sql/driver"
	"encoding/binary"
	"encoding/json"
	"errors"
	"log"
	"strconv"

	bsm "github.com/bitcoin-sv/go-sdk/compat/bsm"
	"github.com/bitcoin-sv/go-sdk/script"
	"github.com/bitcoin-sv/go-sdk/transaction"
	"github.com/bitcoin-sv/go-sdk/util"
	"github.com/shruggr/1sat-indexer/v5/idx"
)

var SIGMA_PROTO = "SIGMA"

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
	idx.BaseIndexer
}

func (i *SigmaIndexer) Tag() string {
	return "sigma"
}

func (i *SigmaIndexer) FromBytes(data []byte) (any, error) {
	var obj Sigmas
	if err := json.Unmarshal(data, &obj); err != nil {
		log.Println("Error unmarshalling map", err)
		return nil, err
	}
	return obj, nil
}

func (i *SigmaIndexer) Parse(idxCtx *idx.IndexContext, vout uint32) (idxData *idx.IndexData) {
	txo := idxCtx.Txos[vout]
	var sigmas Sigmas
	if bitcomData, ok := txo.Data[BITCOM_TAG]; ok {
		for _, b := range bitcomData.Data.([]*Bitcom) {
			if b.Protocol == SIGMA_PROTO {
				sigma := ParseSigma(idxCtx.Tx, vout, b.Pos)
				if sigma != nil {
					sigmas = append(sigmas, sigma)
				}
			}
		}
	}
	if len(sigmas) > 0 {
		idxData = &idx.IndexData{
			Data: sigmas,
		}
	}
	return
}

func ParseSigma(tx *transaction.Transaction, vout uint32, idx int) (sigma *Sigma) {
	// txid := tx.TxID().String()
	// log.Println(txid)
	startIdx := idx
	pos := &idx
	scr := *tx.Outputs[vout].LockingScript
	sigma = &Sigma{}
	for i := 0; i < 4; i++ {
		prevIdx := *pos
		op, err := scr.ReadOp(pos)
		if err != nil || op.Op == script.OpRETURN || (op.Op == 1 && op.Data[0] == '|') {
			*pos = prevIdx
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
			if vin, err := strconv.ParseInt(string(op.Data), 10, 32); err == nil {
				if vin == -1 {
					sigma.Vin = vout
				} else {
					sigma.Vin = uint32(vin)
				}
			}
		}
	}

	outpoint := util.ReverseBytes(tx.Inputs[sigma.Vin].SourceTXID.CloneBytes())
	outpoint = binary.LittleEndian.AppendUint32(outpoint, tx.Inputs[sigma.Vin].SourceTxOutIndex)
	inputHash := sha256.Sum256(outpoint)

	var scriptBuf []byte
	if scr[startIdx-7] == script.OpRETURN {
		scriptBuf = scr[:startIdx-7]
	} else if scr[startIdx-7] == '|' {
		scriptBuf = scr[:startIdx-8]
	} else {
		return nil
	}
	outputHash := sha256.Sum256(scriptBuf)
	msgHash := sha256.Sum256(append(inputHash[:], outputHash[:]...))

	if err := bsm.VerifyMessage(sigma.Address, sigma.Signature, msgHash[:]); err != nil {
		sigma.Valid = false
	} else {
		sigma.Valid = true
	}
	return sigma
}
