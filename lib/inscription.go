package lib

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log"

	"github.com/libsv/go-bt/v2"
	"github.com/libsv/go-bt/v2/bscript"
)

var PATTERN []byte

func init() {
	val, err := hex.DecodeString("0063036f7264")
	if err != nil {
		log.Panic(err)
	}
	PATTERN = val
}

type Inscription struct {
	Body []byte
	Type string
}

func InscriptionFromScript(script bscript.Script) (ins *Inscription, lock []byte) {
	parts, err := bscript.DecodeParts(script)
	if err != nil {
		// log.Panic(err)
		return
	}

	var opfalse int
	var opif int
	var opord int
	lockScript := bscript.Script{}
	for i, op := range parts {
		if len(op) == 1 {
			opcode := op[0]
			if opcode == bscript.Op0 {
				opfalse = i
			}
			if opcode == bscript.OpIF {
				opif = i
			}
			lockScript.AppendOpcodes(opcode)
			continue
		}
		if bytes.Equal(op, []byte("ord")) {
			if opif == i-1 && opfalse == i-2 {
				opord = i
				lockScript = lockScript[:len(lockScript)-2]
				break
			}
		}
		lockScript.AppendPushData(op)
	}

	if opord == 0 {
		hash := sha256.Sum256(script)
		lock = bt.ReverseBytes(hash[:])
		return
	}
	hash := sha256.Sum256(lockScript)
	lock = bt.ReverseBytes(hash[:])
	parts = parts[opord+1:]

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
	Origin  Origin     `json:"origin"`
	Ordinal uint32     `json:"ordinal"`
	Height  uint32     `json:"height"`
	Idx     uint32     `json:"idx"`
	Lock    ByteString `json:"lock"`
}

func (im *InscriptionMeta) Save() (err error) {
	// log.Printf("Saving %x %d %d\n", im.Txid, im.Height, im.Idx)
	_, err = InsInscription.Exec(
		im.Txid,
		im.Vout,
		im.Height,
		im.Idx,
		im.File.Hash,
		im.File.Size,
		im.File.Type,
		im.Origin,
		im.Lock,
	)
	if err != nil {
		log.Panicf("Save Error: %x %d %x %+v\n", im.Txid, im.File.Size, im.File.Type, err)
		log.Panic(err)
	}
	return
}

func GetInsMetaByTxID(txid []byte) (im []*InscriptionMeta, err error) {
	rows, err := Db.Query(`SELECT txid, vout, filehash, filesize, filetype, id, origin, ordinal, height, idx, lock
	FROM inscriptions
	WHERE txid=$1`,
		txid,
	)
	if err != nil {
		return
	}
	defer rows.Close()

	if rows.Next() {
		meta := &InscriptionMeta{}
		err = rows.Scan(
			&meta.Txid,
			&meta.Vout,
			&meta.File.Hash,
			&meta.File.Size,
			&meta.File.Type,
			&meta.Id,
			&meta.Origin,
			&meta.Ordinal,
			&meta.Height,
			&meta.Idx,
			&meta.Lock,
		)
		if err != nil {
			log.Panic(err)
			return
		}
		im = append(im, meta)
	} else {
		err = &HttpError{
			StatusCode: 404,
			Err:        fmt.Errorf("not-found"),
		}
	}
	return
}

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

// func GetInscriptionCount() (count uint64, err error) {
// 	row := GetMaxInscriptionId.QueryRow()
// 	err = row.Scan(&count)
// 	return
// }

// func LoadInscriptionById(id uint64) (im *InscriptionMeta, err error) {
// 	row := GetInsciptionByID.QueryRow(id)
// 	im = &InscriptionMeta{}
// 	err = row.Scan(
// 		&im.Txid,
// 		&im.Vout,
// 		&im.Height,
// 		&im.Idx,
// 		&im.File.Hash,
// 		&im.File.Size,
// 		&im.File.Type,
// 		&im.Id,
// 		&im.Origin,
// 		&im.Lock,
// 	)
// 	return
// }

// func LoadInscription(origin Origin) (im *InscriptionMeta, err error) {
// 	rows, err := GetInsciption.Query(origin)
// 	if err != nil {
// 		return
// 	}
// 	defer rows.Close()

// 	if !rows.Next() {
// 		err = &HttpError{
// 			StatusCode: 404,
// 			Err:        fmt.Errorf("not-found"),
// 		}
// 		return
// 	}
// 	im = &InscriptionMeta{}
// 	err = rows.Scan(
// 		&im.Txid,
// 		&im.Vout,
// 		&im.Height,
// 		&im.Idx,
// 		&im.File.Hash,
// 		&im.File.Size,
// 		&im.File.Type,
// 		&im.Id,
// 		&im.Origin,
// 		&im.Lock,
// 	)
// 	if err != nil {
// 		return
// 	}

// 	return
// }

// func LoadInscriptionsByTxID(txid []byte) (ims []*InscriptionMeta, err error) {
// 	rows, err := GetInsciptionsByTxID.Query(txid)
// 	if err != nil {
// 		return
// 	}
// 	defer rows.Close()

// 	for rows.Next() {
// 		im := &InscriptionMeta{}
// 		err = rows.Scan(
// 			&im.Txid,
// 			&im.Vout,
// 			&im.Height,
// 			&im.Idx,
// 			&im.File.Hash,
// 			&im.File.Size,
// 			&im.File.Type,
// 			&im.Id,
// 			&im.Origin,
// 			&im.Lock,
// 		)
// 		if err != nil {
// 			return
// 		}
// 		ims = append(ims, im)
// 	}
// 	return
// }

// func LoadInscriptions(origin Origin) (ims []*InscriptionMeta, err error) {
// 	rows, err := GetInsciptions.Query(origin)
// 	if err != nil {
// 		return
// 	}
// 	defer rows.Close()

// 	for rows.Next() {
// 		im := &InscriptionMeta{}
// 		err = rows.Scan(
// 			&im.Txid,
// 			&im.Vout,
// 			&im.Height,
// 			&im.Idx,
// 			&im.File.Hash,
// 			&im.File.Size,
// 			&im.File.Type,
// 			&im.Id,
// 			&im.Origin,
// 			&im.Lock,
// 		)
// 		if err != nil {
// 			return
// 		}
// 		ims = append(ims, im)
// 	}

// 	return
// }

// func LoadInscriptionFile(origin Origin) (ins *Inscription, err error) {
// 	im, err := LoadInscription(origin)
// 	if err != nil {
// 		return
// 	}
// 	tx, err := LoadTx(im.Txid)
// 	if err != nil {
// 		return
// 	}

// 	ins, _ = InscriptionFromScript(*tx.Outputs[im.Vout].LockingScript)
// 	return
// }

func SetInscriptionIds(height uint32) (err error) {
	rows, err := GetMaxInscriptionId.Query()
	if err != nil {
		log.Panic(err)
		return
	}
	defer rows.Close()
	var id uint64
	if rows.Next() {
		var dbId sql.NullInt64
		err = rows.Scan(&dbId)
		if err != nil {
			log.Panic(err)
			return
		}
		if dbId.Valid {
			id = uint64(dbId.Int64 + 1)
		}
	} else {
		return
	}

	rows, err = Db.Query(`
		SELECT txid, vout 
		FROM inscriptions
		WHERE id IS NULL AND height <= $1
		ORDER BY height, idx, vout`,
		height,
	)
	if err != nil {
		log.Panic(err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var txid []byte
		var vout uint32
		err = rows.Scan(&txid, &vout)
		if err != nil {
			log.Panic(err)
			return
		}
		fmt.Printf("Inscription ID %d %x %d\n", id, txid, vout)
		_, err = SetInscriptionId.Exec(txid, vout, id)
		if err != nil {
			log.Panic(err)
			return
		}
		id++
	}
	return
}
