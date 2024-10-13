package bopen

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/bitcoin-sv/go-sdk/script"
	"github.com/libsv/go-bt/bscript"
	"github.com/shruggr/1sat-indexer/lib"
)

var INSCRIPTION_TAG = "insc"

type Inscription struct {
	Json    json.RawMessage `json:"json,omitempty"`
	Text    string          `json:"text,omitempty"`
	File    *File           `json:"file,omitempty"`
	Pointer *uint64         `json:"pointer,omitempty"`
	Parent  *lib.Outpoint   `json:"parent,omitempty"`
}

type InscriptionIndexer struct {
	lib.BaseIndexer
}

func (i *InscriptionIndexer) Tag() string {
	return INSCRIPTION_TAG
}

func (i *InscriptionIndexer) FromBytes(data []byte) (any, error) {
	obj := &Inscription{}
	if err := json.Unmarshal(data, obj); err != nil {
		return nil, err
	}
	return obj, nil
}

func (i *InscriptionIndexer) Parse(idxCtx *lib.IndexContext, vout uint32) (idxData *lib.IndexData) {
	txo := idxCtx.Txos[vout]
	if bopen, ok := txo.Data[BOPEN_TAG]; ok {
		if insc, ok := bopen.Data.(BOpen)[INSCRIPTION_TAG].(*Inscription); ok {
			idxData = &lib.IndexData{
				Data: insc,
				Events: []*lib.Event{
					{
						Id:    "type",
						Value: insc.File.Type,
					},
				},
			}
		}
	}
	return
}

func (i *InscriptionIndexer) PreSave(idxCtx *lib.IndexContext) {
	for _, txo := range idxCtx.Txos {
		if bopen, ok := txo.Data[BOPEN_TAG]; ok {
			if insc, ok := bopen.Data.(BOpen)[i.Tag()].(*Inscription); ok {
				insc.File.Content = nil
			}
		}
	}
}

func ParseInscription(txo *lib.Txo, scr *script.Script, fromPos *int, bopen BOpen) *Inscription {
	insc := &Inscription{
		File: &File{},
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
			if string(op.Data) == MAP {
				scr := script.NewFromBytes(op2.Data)
				pos := 0
				md := ParseMAP(scr, &pos)
				bopen.addInstance(md)
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
		case 2:
			var pointer uint64
			if len(op2.Data) > 0 {
				pointer = binary.LittleEndian.Uint64(op2.Data)
			}
			insc.Pointer = &pointer
		case 3:
			insc.Parent = lib.NewOutpointFromBytes(op2.Data)
		case 5:
		// 	md := &Map{}
		// 	if err := cbor.Unmarshal(op2.Data, md); err == nil {
		// 		insc.Metadata = *md
		// 	}
		case 9:
			insc.File.Encoding = string(op2.Data)
		}

	}
	op, err := lib.ReadOp(*scr, &pos)
	if err != nil || op.OpCode != script.OpENDIF {
		return insc
	}
	*fromPos = pos

	insc.File.Size = uint32(len(insc.File.Content))
	hash := sha256.Sum256(insc.File.Content)
	insc.File.Hash = hash[:]
	insType := "file"
	// var bsv20 *Bsv20
	if insc.File.Size <= 1024 && utf8.Valid(insc.File.Content) && !bytes.Contains(insc.File.Content, []byte{0}) && !bytes.Contains(insc.File.Content, []byte("\\u0000")) {
		mime := strings.ToLower(insc.File.Type)
		if strings.HasPrefix(mime, "application") || strings.HasPrefix(mime, "text") {
			var data json.RawMessage
			if err := json.Unmarshal(insc.File.Content, &data); err == nil {
				insType = "json"
				insc.Json = data
			} else if AsciiRegexp.Match(insc.File.Content) {
				if insType == "file" {
					insType = "text"
				}
				insc.Text = string(insc.File.Content)
				// re := regexp.MustCompile(`\W`)
				// words := map[string]struct{}{}
				// for _, word := range re.Split(insc.Text, -1) {
				// 	if len(word) > 0 {
				// 		word = strings.ToLower(word)
				// 		words[word] = struct{}{}
				// 	}
				// }
				// if len(words) > 0 {
				// 	insc.Words = make([]string, 0, len(words))
				// 	for word := range words {
				// 		insc.Words = append(insc.Words, word)
				// 	}
				// }
			}
		}
	}

	if len(*scr) >= pos+25 && bscript.NewFromBytes((*scr)[pos:pos+25]).IsP2PKH() {
		pkhash := lib.PKHash((*scr)[pos+3 : pos+23])
		txo.AddOwner(pkhash.Address())
	} else if len(*scr) >= pos+26 &&
		(*scr)[pos] == bscript.OpCODESEPARATOR &&
		script.NewFromBytes((*scr)[pos+1:pos+26]).IsP2PKH() {
		pkhash := lib.PKHash((*scr)[pos+4 : pos+24])
		txo.AddOwner(pkhash.Address())
	}

	return insc
}
