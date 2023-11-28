package ordinals

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/libsv/go-bt/bscript"
	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/lib"
)

var AsciiRegexp = regexp.MustCompile(`^[[:ascii:]]*$`)

type Inscription struct {
	Json  json.RawMessage `json:"json,omitempty"`
	Text  string          `json:"text,omitempty"`
	Words []string        `json:"words,omitempty"`
	File  *lib.File       `json:"file,omitempty"`
}

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
		if txo.Origin == nil || txo.Data == nil {
			continue
		}

		if _, ok := txo.Data["insc"]; ok && bytes.Equal(*txo.Origin, *txo.Outpoint) {
			origin := &Origin{
				Origin: txo.Outpoint,
				Height: ctx.Height,
				Idx:    ctx.Idx,
			}
			if Map, ok := txo.Data["map"].(lib.Map); ok {
				origin.Map = Map
			}
			origin.Save()
		} else if txo.Data["map"] != nil {
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

	var opFalse int
	var opIf int
	var opReturn int
	for i := start; i < len(script); {
		startI := i
		op, err := lib.ReadOp(script, &i)
		if err != nil {
			break
		}
		switch op.OpCode {
		case bscript.Op0:
			opFalse = startI
		case bscript.OpIF:
			opIf = startI
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
		}

		if bytes.Equal(op.Data, []byte("ord")) && opIf == startI-1 && opFalse == startI-2 {
			ins := &Inscription{
				File: &lib.File{},
			}
		ordLoop:
			for {
				op, err = lib.ReadOp(script, &i)
				if err != nil {
					break
				}
				switch op.OpCode {
				case bscript.Op0:
					op, err = lib.ReadOp(script, &i)
					if err != nil {
						break ordLoop
					}
					ins.File.Content = op.Data
				case bscript.Op1:
					op, err = lib.ReadOp(script, &i)
					if err != nil {
						break ordLoop
					}
					if utf8.Valid(op.Data) {
						if len(op.Data) <= 256 {
							ins.File.Type = string(op.Data)
						} else {
							ins.File.Type = string(op.Data[:256])
						}
					}
				case bscript.OpENDIF:
					break ordLoop
				}
			}

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
					err = json.Unmarshal(ins.File.Content, &data)
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
							word = strings.ToLower(word)
							if len(word) > 1 {
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

			if len(txo.PKHash) == 0 && len(script) >= i+25 && bscript.NewFromBytes(script[i:i+25]).IsP2PKH() {
				txo.PKHash = []byte(script[i+3 : i+23])
			}
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
