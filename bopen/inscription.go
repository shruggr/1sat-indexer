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

type Inscription struct {
	Json json.RawMessage `json:"json,omitempty"`
	Text string          `json:"text,omitempty"`
	// Words   []string        `json:"words,omitempty"`
	File    *File         `json:"file,omitempty"`
	Pointer *uint64       `json:"pointer,omitempty"`
	Parent  *lib.Outpoint `json:"parent,omitempty"`
	// Metadata  Map         `json:"metadata,omitempty"`
	// Metaproto []byte          `json:"metaproto,omitempty"`
	// Fields    Map         `json:"-"`
}

type InscriptionIndexer struct {
	lib.BaseIndexer
}

func (i *InscriptionIndexer) Tag() string {
	return "insc"
}

func (i *InscriptionIndexer) Parse(idxCtx *lib.IndexContext, vout uint32) (idxData *lib.IndexData) {
	txo := idxCtx.Txos[vout]
	if bopen, ok := txo.Data[BOPEN]; ok {
		if bopenData, ok := bopen.Data.(map[string]any); ok {
			if insc, ok := bopenData[i.Tag()].(*Inscription); ok {
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
	}
	return
}

func (i *InscriptionIndexer) PreSave(idxCtx *lib.IndexContext) {
	for _, txo := range idxCtx.Txos {
		if bopen, ok := txo.Data[BOPEN]; ok {
			if bopenData, ok := bopen.Data.(map[string]any); ok {
				if insc, ok := bopenData[i.Tag()].(*Inscription); ok {
					insc.File.Content = nil
				}
			}
		}
	}
}

func ParseInscription(txo *lib.Txo, scr *script.Script, fromPos *int) *lib.IndexData {
	insc := &Inscription{
		File: &File{},
	}
	idxData := &lib.IndexData{
		Data: insc,
	}
	pos := *fromPos

ordLoop:
	for {
		var field int
		if op, err := scr.ReadOp(&pos); err != nil || op.Op > script.Op16 {
			return idxData
		} else if op2, err := scr.ReadOp(&pos); err != nil || op2.Op > script.Op16 {
			return idxData
		} else if op.Op > script.OpPUSHDATA4 && op.Op <= script.Op16 {
			field = int(op.Op) - 80
		} else if len(op.Data) == 1 {
			field = int(op.Data[0])
		} else if len(op.Data) > 1 {
			// if insc.Fields == nil {
			// 	insc.Fields = Map{}
			// }

			// if len(op.Data) <= 64 && utf8.Valid(op.Data) {
			// 	opKey := strings.Replace(string(bytes.Replace(op.Data, []byte{0}, []byte{' '}, -1)), "\\u0000", " ", -1)
			// 	insc.Fields[opKey] = strings.Replace(string(bytes.Replace(op2.Data, []byte{0}, []byte{' '}, -1)), "\\u0000", " ", -1)
			// }
			if string(op.Data) == MAP {
				scr := script.NewFromBytes(op2.Data)
				pos := 0
				md := ParseMAP(scr, &pos)
				addInstance(txo, md)
			}
			continue
		} else {
			switch field {
			case 0:
				insc.File.Content = op2.Data
				break ordLoop
			case 1:
				if len(op2.Data) < 256 && utf8.Valid(op2.Data) {
					insc.File.Type = string(op2.Data)
					idxData.Events = append(idxData.Events, &lib.Event{
						Id:    "type",
						Value: insc.File.Type,
					})
				}
			case 2:
				var pointer uint64
				if len(op2.Data) > 0 {
					pointer = binary.LittleEndian.Uint64(op2.Data)
				}
				insc.Pointer = &pointer
			case 3:
				parent := lib.Outpoint(op2.Data)
				insc.Parent = &parent
			// case 5:
			// 	md := &Map{}
			// 	if err := cbor.Unmarshal(op2.Data, md); err == nil {
			// 		insc.Metadata = *md
			// 	}
			// case 7:
			// 	insc.Metaproto = op2.Data
			case 9:
				insc.File.Encoding = string(op2.Data)
			}
		}

	}
	op, err := lib.ReadOp(*scr, &pos)
	if err != nil || op.OpCode != script.OpENDIF {
		return idxData
	}
	*fromPos = pos

	insc.File.Size = uint32(len(insc.File.Content))
	hash := sha256.Sum256(insc.File.Content)
	insc.File.Hash = hash[:]
	insType := "file"
	// var bsv20 *Bsv20
	if insc.File.Size <= 1024 && utf8.Valid(insc.File.Content) && !bytes.Contains(insc.File.Content, []byte{0}) && !bytes.Contains(insc.File.Content, []byte("\\u0000")) {
		mime := strings.ToLower(insc.File.Type)
		if strings.HasPrefix(mime, "application/json") ||
			strings.HasPrefix(mime, "text") {

			var data json.RawMessage
			if err := json.Unmarshal(insc.File.Content, &data); err == nil {
				insType = "json"
				insc.Json = data
				// bsv20, _ = ParseBsv20Inscription(insc.File, txo)
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

	return idxData
}

// func (i *Inscription) Save() {
// 	_, err := lib.Db.Exec(context.Background(), `
// 		INSERT INTO inscriptions(outpoint, height, idx)
// 		VALUES($1, $2, $3)
// 		ON CONFLICT(outpoint) DO UPDATE SET
// 			height=EXCLUDED.height,
// 			idx=EXCLUDED.idx`,
// 		i.Outpoint,
// 		i.Height,
// 		i.Idx,
// 	)
// 	if err != nil {
// 		log.Panicf("Save Error: %s %+v\n", i.Outpoint, err)
// 	}
// }

// func SetInscriptionNum(height uint32) (err error) {
// 	row := lib.Db.QueryRow(context.Background(),
// 		"SELECT MAX(num) FROM inscriptions",
// 	)
// 	var dbNum sql.NullInt64
// 	err = row.Scan(&dbNum)
// 	if err != nil {
// 		log.Panic(err)
// 		return
// 	}
// 	var num uint64
// 	if dbNum.Valid {
// 		num = uint64(dbNum.Int64 + 1)
// 	}

// 	rows, err := lib.Db.Query(context.Background(), `
// 		SELECT outpoint
// 		FROM inscriptions
// 		WHERE num = -1 AND height <= $1 AND height IS NOT NULL
// 		ORDER BY height, idx
// 		LIMIT 100000`,
// 		height,
// 	)
// 	if err != nil {
// 		log.Panic(err)
// 		return
// 	}
// 	defer rows.Close()
// 	for rows.Next() {
// 		outpoint := &lib.Outpoint{}
// 		err = rows.Scan(&outpoint)
// 		if err != nil {
// 			log.Panic(err)
// 			return
// 		}
// 		// fmt.Printf("Inscription Num %d %d %s\n", num, height, outpoint)
// 		_, err = lib.Db.Exec(context.Background(), `
// 			UPDATE inscriptions
// 			SET num=$2
// 			WHERE outpoint=$1`,
// 			outpoint, num,
// 		)
// 		if err != nil {
// 			log.Panic(err)
// 			return
// 		}
// 		num++
// 	}
// 	lib.PublishEvent(context.Background(), "inscriptionNum", fmt.Sprintf("%d", num))
// 	// log.Println("Height", height, "Max Origin Num", num)
// 	return
// }
