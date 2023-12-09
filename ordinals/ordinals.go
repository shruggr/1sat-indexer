package ordinals

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/fxamacker/cbor"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/libsv/go-bt/bscript"
	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/lib"
)

var AsciiRegexp = regexp.MustCompile(`^[[:ascii:]]*$`)
var Db *pgxpool.Pool
var Rdb *redis.Client

func Initialize(db *pgxpool.Pool, rdb *redis.Client) (err error) {
	Db = db
	Rdb = rdb

	lib.Initialize(db, rdb)
	return
}

func IndexTxn(rawtx []byte, blockId string, height uint32, idx uint64, dryRun bool) (ctx *lib.IndexContext) {
	ctx, err := lib.IndexTxn(rawtx, blockId, height, idx, dryRun)
	if err != nil {
		panic(err)
	}
	IndexInscriptions(ctx)
	return
}

func IndexInscriptions(ctx *lib.IndexContext) {
	for _, txo := range ctx.Txos {
		if len(txo.PKHash) != 0 {
			continue
		}
		ParseScript(txo)
		if txo.Satoshis == 1 {
			txo.Origin = LoadOrigin(txo.Outpoint, txo.OutAcc)
		}
		txo.Save()
		if insc, ok := txo.Data["insc"].(*Inscription); ok && ctx.Height != nil {
			insc.Outpoint = txo.Outpoint
			insc.Height = ctx.Height
			insc.Idx = ctx.Idx
			insc.Save()
		}
		if txo.Origin == nil || txo.Data == nil {
			continue
		}

		if txo.Data["map"] != nil {
			SaveMap(txo.Origin)
		}
		if bsv20, ok := txo.Data["bsv20"].(*Bsv20); ok {
			bsv20.Save(txo)
		}
	}

	Db.Exec(context.Background(),
		`INSERT INTO txn_indexer(txid, indexer) 
		VALUES ($1, 'ord')
		ON CONFLICT DO NOTHING`,
		ctx.Txid,
	)
}

func ParseScript(txo *lib.Txo) {
	vout := txo.Outpoint.Vout()
	script := *txo.Tx.Outputs[vout].LockingScript

	start := 0
	if len(script) >= 25 && bscript.NewFromBytes(script[:25]).IsP2PKH() {
		txo.PKHash = []byte(script[3:23])
		start = 25
	}

	var opReturn int
	for i := start; i < len(script); {
		startI := i
		op, err := lib.ReadOp(script, &i)
		if err != nil {
			break
		}
		switch op.OpCode {
		case bscript.OpRETURN:
			if opReturn == 0 {
				opReturn = startI
			}
			bitcom, err := lib.ParseBitcom(txo.Tx, vout, &i)
			if err != nil {
				continue
			}
			addBitcom(txo, bitcom)

		case bscript.OpDATA1:
			if op.Data[0] == '|' && opReturn > 0 {
				bitcom, err := lib.ParseBitcom(txo.Tx, vout, &i)
				if err != nil {
					continue
				}
				addBitcom(txo, bitcom)
			}
		case bscript.OpDATA3:
			if i > 2 && bytes.Equal(op.Data, []byte("ord")) && script[startI-2] == 0 && script[startI-1] == bscript.OpIF {
				ParseInscription(txo, script, &i)
			}
		}
	}
}

func ParseInscription(txo *lib.Txo, script []byte, fromPos *int) {
	pos := *fromPos
	ins := &Inscription{
		File: &lib.File{},
	}

ordLoop:
	for {
		op, err := lib.ReadOp(script, &pos)
		if err != nil || op.OpCode > bscript.Op16 {
			return
		}

		op2, err := lib.ReadOp(script, &pos)
		if err != nil || op2.OpCode > bscript.Op16 {
			return
		}

		var field int
		if op.OpCode > bscript.OpPUSHDATA4 && op.OpCode <= bscript.Op16 {
			field = int(op.OpCode) - 80
		} else if len(op.Data) > 1 {
			continue
		} else if op.OpCode != bscript.Op0 {
			field = int(op.Data[0])
		}

		switch field {
		case 0:
			op, err := lib.ReadOp(script, &pos)
			ins.File.Content = op2.Data
			if err != nil || op.OpCode != bscript.OpENDIF {
				return
			}
			break ordLoop
		case 1:
			ins.File.Type = string(op2.Data)
		case 2:
			ins.File.Type = string(op2.Data)
		case 3:
			pointer := binary.LittleEndian.Uint64(op2.Data)
			ins.Pointer = &pointer
		case 5:
			md := &lib.Map{}
			if err := cbor.Unmarshal(op2.Data, md); err == nil {
				ins.Metadata = *md
			}
		case 7:
			ins.Metaproto = op2.Data
		case 9:
			ins.File.Encoding = string(op2.Data)
		}
	}
	*fromPos = pos

	ins.File.Size = uint32(len(ins.File.Content))
	hash := sha256.Sum256(ins.File.Content)
	ins.File.Hash = hash[:]
	insType := "file"
	var bsv20 *Bsv20
	if ins.File.Size <= 1024 && utf8.Valid(ins.File.Content) && !bytes.Contains(ins.File.Content, []byte{0}) {
		mime := strings.ToLower(ins.File.Type)
		if strings.HasPrefix(mime, "application") ||
			strings.HasPrefix(mime, "text") {

			var data json.RawMessage
			err := json.Unmarshal(ins.File.Content, &data)
			if err == nil {
				insType = "json"
				ins.Json = data
				bsv20, _ = ParseBsv20(ins.File, txo.Height)
			} else if AsciiRegexp.Match(ins.File.Content) {
				if insType == "file" {
					insType = "text"
				}
				ins.Text = string(ins.File.Content)
				re := regexp.MustCompile(`\W`)
				words := map[string]struct{}{}
				for _, word := range re.Split(ins.Text, -1) {
					if len(word) > 0 {
						word = strings.ToLower(word)
						words[word] = struct{}{}
					}
				}
				if len(words) > 0 {
					ins.Words = make([]string, 0, len(words))
					for word := range words {
						ins.Words = append(ins.Words, word)
					}
				}
			}
		}
	}
	if txo.Data == nil {
		txo.Data = map[string]interface{}{}
	}
	txo.Data["insc"] = ins
	var types []string
	if prev, ok := txo.Data["types"].([]string); ok {
		types = prev
	}
	types = append(types, insType)
	txo.Data["types"] = types
	if bsv20 != nil {
		txo.Data["bsv20"] = bsv20
	}

	if len(txo.PKHash) == 0 {
		if len(script) >= pos+25 && bscript.NewFromBytes(script[pos:pos+25]).IsP2PKH() {
			txo.PKHash = []byte(script[pos+3 : pos+23])
		} else if len(script) >= pos+26 &&
			script[pos] == bscript.OpCODESEPARATOR &&
			bscript.NewFromBytes(script[pos+1:pos+26]).IsP2PKH() {

			txo.PKHash = []byte(script[pos+4 : pos+24])
		}
	}
}

func addBitcom(txo *lib.Txo, bitcom interface{}) {
	if bitcom == nil {
		return
	}
	switch bc := bitcom.(type) {
	case *lib.Sigma:
		var sigmas []*lib.Sigma
		if prev, ok := txo.Data["sigma"].([]*lib.Sigma); ok {
			sigmas = prev
		}
		sigmas = append(sigmas, bc)
		txo.AddData("sigma", sigmas)
	case lib.Map:
		txo.AddData("map", bc)
	case *lib.File:
		txo.AddData("b", bc)
	}
}
