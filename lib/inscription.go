package lib

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log"

	"github.com/libsv/go-bt/v2/bscript"
)

var PATTERN []byte
var insInscription *sql.Stmt

func init() {
	val, err := hex.DecodeString("0063036f7264")
	if err != nil {
		log.Panic(err)
	}
	PATTERN = val

	insInscription, err = Db.Prepare(`
		INSERT INTO inscriptions(txid, vout, height, idx, filehash, filesize, filetype, origin, lock)
			VALUES($1, $2, $3, $4, $5, $6, $7, $8, $9)
			ON CONFLICT(txid, vout) DO UPDATE
				SET height=EXCLUDED.height, idx=EXCLUDED.idx
	`)
	if err != nil {
		log.Panic(err)
	}
}

type Inscription struct {
	Body []byte
	Type string
}

func InscriptionFromScript(script bscript.Script) (ins *Inscription, lock [32]byte) {
	parts, err := bscript.DecodeParts(script)
	if err != nil {
		// log.Panic(err)
		return
	}

	var opfalse int
	var opif int
	var opord int

	for i, op := range parts {
		if len(op) == 1 {
			opcode := op[0]
			if opcode == bscript.Op0 {
				opfalse = i
				continue
			}
			if opcode == bscript.OpIF {
				opif = i
				continue
			}
		}
		if bytes.Equal(op, []byte("ord")) {
			if opif == i-1 && opfalse == i-2 {
				opord = i
				break
			}
		}
	}

	if opord == 0 {
		lock = sha256.Sum256(script)
		return
	}
	parts = parts[opord+1:]
	lock = sha256.Sum256(script[:opif])

	ins = &Inscription{}
	for i := 0; i < len(parts); i++ {
		op := parts[i]
		if len(op) != 1 {
			break
		}
		opcode := op[0]
		switch opcode {
		case bscript.Op0:
			value := parts[i+1]
			if len(value) == 1 && value[0] == bscript.Op0 {
				value = []byte{}
			}
			ins.Body = value
			return
		case bscript.Op1:
			value := parts[i+1]
			if len(value) == 1 && value[0] == bscript.Op0 {
				value = []byte{}
			}
			ins.Type = string(value)
		case bscript.OpENDIF:
			return
		}
		i++
	}
	return
}

type File struct {
	Hash ByteString `json:"hash"`
	Size uint32     `json:"size"`
	Type string     `json:"type"`
}

type InscriptionMeta struct {
	Id      uint64     `json:"id"`
	Txid    ByteString `json:"txid"`
	Vout    uint32     `json:"vout"`
	File    File       `json:"file"`
	Origin  ByteString `json:"origin"`
	Ordinal uint32     `json:"ordinal"`
	Height  uint32     `json:"height"`
	Idx     uint32     `json:"idx"`
	Lock    []byte     `json:"lock"`
}

func (im *InscriptionMeta) Save() (err error) {
	// log.Printf("Saving %x %d %d\n", im.Txid, im.Height, im.Idx)
	_, err = insInscription.Exec(
		im.Txid,
		im.Vout,
		im.Height,
		im.Idx,
		im.File.Hash,
		im.File.Size,
		im.File.Type,
		im.Origin,
		im.Lock[:],
	)
	if err != nil {
		log.Panicf("Save Error: %x %d %x %+v\n", im.Txid, im.File.Size, im.File.Type, err)
		log.Panic(err)
	}
	return
}

// func ProcessInsTx(tx *bt.Tx, height uint32, idx uint32) (inscriptions []*InscriptionMeta, err error) {
// 	txid := tx.TxIDBytes()
// 	for vout, txout := range tx.Outputs {
// 		var im *InscriptionMeta
// 		im, err = ProcessInsOutput(txid, uint32(vout), txout, height, idx)
// 		if err != nil {
// 			return
// 		}
// 		if im != nil {
// 			inscriptions = append(inscriptions, im)
// 		}
// 	}

// 	return
// }

// func ProcessInsOutput(txid []byte, vout uint32, txout *bt.Output, height uint32, idx uint32) (ins *InscriptionMeta, err error) {
// 	inscription, lock := InscriptionFromScript(*txout.LockingScript)
// 	if inscription == nil {
// 		fmt.Printf("Not an inscription: %x %d\n", txid, vout)
// 		return
// 	}

// 	hash := sha256.Sum256(inscription.Body)

// 	im := &InscriptionMeta{
// 		Txid: txid,
// 		Vout: uint32(vout),
// 		File: File{
// 			Hash: hash[:],
// 			Size: uint32(len(inscription.Body)),
// 			Type: inscription.Type,
// 		},
// 		Height: height,
// 		Idx:    idx,
// 		Lock:   lock,
// 	}

// 	err = im.Save()
// 	if err != nil {
// 		log.Panic(err)
// 		return
// 	}
// 	return
// }

func GetInsMetaByOutpoint(txid []byte, vout uint32) (im *InscriptionMeta, err error) {
	rows, err := Db.Query(`SELECT txid, vout, filehash, filesize, filetype, id, origin, ordinal, height, idx, lock
		FROM inscriptions
		WHERE txid=$1 AND vout=$2`,
		txid,
		vout,
	)
	if err != nil {
		return
	}
	defer rows.Close()

	if rows.Next() {
		im = &InscriptionMeta{}
		err = rows.Scan(
			&im.Txid,
			&im.Vout,
			&im.File.Hash,
			&im.File.Size,
			&im.File.Type,
			&im.Id,
			&im.Origin,
			&im.Ordinal,
			&im.Height,
			&im.Idx,
			&im.Lock,
		)
		if err != nil {
			log.Panic(err)
			return
		}
	} else {
		err = &HttpError{
			StatusCode: 404,
			Err:        fmt.Errorf("not-found"),
		}
	}
	return
}

func GetInsMetaByOrigin(origin []byte) (ins []*InscriptionMeta, err error) {
	rows, err := Db.Query(`SELECT txid, vout, filehash, filesize, filetype, id, origin, ordinal, height, idx, lock
		FROM inscriptions
		WHERE origin=$1
		ORDER BY height DESC, idx DESC`,
		origin,
	)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		im := &InscriptionMeta{}
		err = rows.Scan(
			&im.Txid,
			&im.Vout,
			&im.File.Hash,
			&im.File.Size,
			&im.File.Type,
			&im.Id,
			&im.Origin,
			&im.Ordinal,
			&im.Height,
			&im.Idx,
			&im.Lock,
		)
		if err != nil {
			log.Panic(err)
			return
		}

		ins = append(ins, im)
	}
	return
}

func LoadInsByOrigin(origin []byte) (ins *Inscription, err error) {
	rows, err := Db.Query(`SELECT txid, vout, filetype
		FROM inscriptions
		WHERE origin=$1
		ORDER BY height DESC, idx DESC
		LIMIT 1`,
		origin,
	)
	if err != nil {
		return
	}
	defer rows.Close()

	if !rows.Next() {
		err = &HttpError{
			StatusCode: 404,
			Err:        fmt.Errorf("not-found"),
		}
		return
	}
	var txid []byte
	var vout uint32
	var filetype string
	err = rows.Scan(&txid, &vout, &filetype)
	if err != nil {
		return
	}

	tx, err := LoadTx(txid)
	if err != nil {
		return
	}

	ins, _ = InscriptionFromScript(*tx.Outputs[vout].LockingScript)
	return
}
