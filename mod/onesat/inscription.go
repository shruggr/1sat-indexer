package onesat

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/bitcoin-sv/go-sdk/script"
	"github.com/shruggr/1sat-indexer/v5/evt"
	"github.com/shruggr/1sat-indexer/v5/idx"
	"github.com/shruggr/1sat-indexer/v5/lib"
	"github.com/shruggr/1sat-indexer/v5/mod/bitcom"
)

var INSC_TAG = "insc"

var AsciiRegexp = regexp.MustCompile(`^[[:ascii:]]*$`)

type Inscription struct {
	Json    json.RawMessage   `json:"json,omitempty"`
	JsonMap map[string]string `json:"-"`
	Text    string            `json:"text,omitempty"`
	File    *lib.File         `json:"file,omitempty"`
	Pointer *uint64           `json:"pointer,omitempty"`
	Parent  *lib.Outpoint     `json:"parent,omitempty"`
}

type InscriptionIndexer struct {
	idx.BaseIndexer
}

func (i *InscriptionIndexer) Tag() string {
	return INSC_TAG
}

func (i *InscriptionIndexer) FromBytes(data []byte) (any, error) {
	obj := &Inscription{}
	if err := json.Unmarshal(data, obj); err != nil {
		return nil, err
	}
	return obj, nil
}

func (i *InscriptionIndexer) Parse(idxCtx *idx.IndexContext, vout uint32) *idx.IndexData {
	txo := idxCtx.Txos[vout]
	if idxCtx.Height < TRIGGER || txo.Satoshis == nil || *txo.Satoshis != 1 {
		return nil
	}
	scr := idxCtx.Tx.Outputs[vout].LockingScript

	for i := 0; i < len(*scr); {
		startI := i
		if op, err := scr.ReadOp(&i); err != nil {
			break
		} else if i > 2 && op.Op == script.OpDATA3 && bytes.Equal(op.Data, []byte("ord")) && (*scr)[startI-2] == 0 && (*scr)[startI-1] == script.OpIF {
			insc := ParseInscription(txo, scr, &i, txo.Data[bitcom.BITCOM_TAG])
			if insc != nil {

				idxCtx := &idx.IndexData{
					Data: insc,
				}
				if insc.File != nil {
					parts := strings.Split(insc.File.Type, ";")
					if len(parts) > 0 {
						idxCtx.Events = append(idxCtx.Events, &evt.Event{
							Id:    "type",
							Value: parts[0],
						})
					}
				}
				return idxCtx
			}
			break
		}
	}
	return nil
}

func (i *InscriptionIndexer) PreSave(idxCtx *idx.IndexContext) {
	for _, txo := range idxCtx.Txos {
		if inscData, ok := txo.Data[INSC_TAG]; ok {
			if insc, ok := inscData.Data.(*Inscription); ok {
				insc.File.Content = nil
			}
		}
	}
}

func ParseInscription(txo *idx.Txo, scr *script.Script, fromPos *int, bitcomData *idx.IndexData) *Inscription {
	insc := &Inscription{
		File: &lib.File{},
	}
	pos := *fromPos

ordLoop:
	for {
		var field int
		var err error
		var op, op2 *script.ScriptChunk
		if op, err = scr.ReadOp(&pos); err != nil || op.Op > script.Op16 {
			return insc
		} else if op2, err = scr.ReadOp(&pos); err != nil || op2.Op > script.Op16 {
			return insc
		} else if op.Op > script.OpPUSHDATA4 && op.Op <= script.Op16 {
			field = int(op.Op) - 80
		} else if len(op.Data) == 1 {
			field = int(op.Data[0])
		} else if len(op.Data) > 1 {
			if bitcomData != nil {
				b := append(bitcomData.Data.([]*bitcom.Bitcom), &bitcom.Bitcom{
					Protocol: string(op.Data),
					Script:   op2.Data,
					Pos:      pos,
				})
				bitcomData.Data = b
			}

			continue
		}
		switch field {
		case 0:
			insc.File.Content = op2.Data
			break ordLoop
		case 1:
			if len(op2.Data) < 256 && utf8.Valid(op2.Data) {
				insc.File.Type = string(op2.Data)
			}
		// case 2:
		// 	var pointer uint64
		// 	if len(op2.Data) > 0 {
		// 		pointer = binary.LittleEndian.Uint64(op2.Data)
		// 	}
		// 	insc.Pointer = &pointer
		case 3:
			insc.Parent = lib.NewOutpointFromBytes(op2.Data)
		case 9:
			insc.File.Encoding = string(op2.Data)
		}

	}
	op, err := scr.ReadOp(&pos)
	if err != nil || op.Op != script.OpENDIF {
		return insc
	}
	*fromPos = pos

	insc.File.Size = uint32(len(insc.File.Content))
	hash := sha256.Sum256(insc.File.Content)
	insc.File.Hash = hash[:]
	insType := "file"
	if insc.File.Size <= 1024 && utf8.Valid(insc.File.Content) && !bytes.Contains(insc.File.Content, []byte{0}) && !bytes.Contains(insc.File.Content, []byte("\\u0000")) {
		mime := strings.ToLower(insc.File.Type)
		if strings.HasPrefix(mime, "application/json") || strings.HasPrefix(mime, "text") {
			if err := json.Unmarshal(insc.File.Content, &insc.Json); err == nil {
				insType = "json"
			} else if AsciiRegexp.Match(insc.File.Content) {
				if insType == "file" {
					insType = "text"
				}
				insc.Text = string(insc.File.Content)
			}
		}
		if strings.HasPrefix(insc.File.Type, "application/bsv-20") {
			insc.JsonMap = map[string]string{}
			json.Unmarshal(insc.File.Content, &insc.JsonMap)
		}
	}

	if len(*scr) >= pos+25 && script.NewFromBytes((*scr)[pos:pos+25]).IsP2PKH() {
		pkhash := lib.PKHash((*scr)[pos+3 : pos+23])
		txo.AddOwner(pkhash.Address())
	} else if len(*scr) >= pos+26 &&
		(*scr)[pos] == script.OpCODESEPARATOR &&
		script.NewFromBytes((*scr)[pos+1:pos+26]).IsP2PKH() {
		pkhash := lib.PKHash((*scr)[pos+4 : pos+24])
		txo.AddOwner(pkhash.Address())
	}

	return insc
}
