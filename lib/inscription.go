package lib

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"database/sql/driver"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"

	"github.com/libsv/go-bt/v2"
	"github.com/libsv/go-bt/v2/bscript"
)

var PATTERN []byte
var MAP = []byte("1PuQa7K62MiKCtssSLKy1kh56WWU7MtUR5")

type Map map[string]string

func (m Map) Value() (driver.Value, error) {
	return json.Marshal(m)
}

func (m *Map) Scan(value interface{}) error {
	b, ok := value.([]byte)
	if !ok {
		return errors.New("type assertion to []byte failed")
	}
	return json.Unmarshal(b, &m)
}

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

type File struct {
	Hash ByteString `json:"hash"`
	Size uint32     `json:"size"`
	Type string     `json:"type"`
}

type ParsedScript struct {
	Id          uint64       `json:"id"`
	Txid        ByteString   `json:"txid"`
	Vout        uint32       `json:"vout"`
	File        File         `json:"file"`
	Origin      Origin       `json:"origin"`
	Ordinal     uint32       `json:"ordinal"`
	Height      uint32       `json:"height"`
	Idx         uint32       `json:"idx"`
	Lock        ByteString   `json:"lock"`
	Map         Map          `json:"MAP"`
	Inscription *Inscription `json:"-"`
}

func (p *ParsedScript) Save() (err error) {
	// var MAP []byte
	// if p.Map != nil {
	// 	MAP, err = json.Marshal(p.Map)
	// 	if err != nil {
	// 		return
	// 	}
	// }
	if p.Inscription != nil {
		_, err = InsInscription.Exec(
			p.Txid,
			p.Vout,
			p.Height,
			p.Idx,
			p.File.Hash,
			p.File.Size,
			p.File.Type,
			p.Map,
			p.Origin,
			p.Lock,
		)
		if err != nil {
			log.Panicf("Save Error: %x %d %x %+v\n", p.Txid, p.File.Size, p.File.Type, err)
			log.Panic(err)
		}
	}

	if p.Inscription != nil || p.Map != nil {
		_, err = InsMetadata.Exec(
			p.Txid,
			p.Vout,
			p.Height,
			p.Idx,
			p.File.Hash,
			p.File.Size,
			p.File.Type,
			p.Map,
			p.Origin,
		)
		if err != nil {
			log.Panicf("Save Error: %x %d %x %+v\n", p.Txid, p.File.Size, p.File.Type, err)
			log.Panic(err)
		}
	}
	return
}

func ParseScript(script bscript.Script, includeFileMeta bool) (p *ParsedScript) {
	parts, err := bscript.DecodeParts(script)
	if err != nil {
		// log.Panic(err)
		return
	}

	var opFalse int
	var opIf int
	var opORD int
	var opMAP int
	var endLock int
	var mapOperator string
	lockScript := bscript.Script{}

parseLoop:
	for i, op := range parts {
		var opcode byte
		if len(op) == 1 {
			opcode = op[0]
			switch opcode {
			case bscript.Op0:
				opFalse = i
			case bscript.OpIF:
				opIf = i
			case bscript.OpRETURN:
				if endLock == 0 {
					endLock = i
				}
				if opORD == 0 {
					opORD = -1
				}
				if len(parts) > i+4 && bytes.Equal(parts[i+1], MAP) {
					mapOperator = string(parts[i+2])
					opMAP = i + 3
					break parseLoop
				}
			case bscript.OpSWAP:
				if endLock > 0 {
					if len(parts) > i+4 && bytes.Equal(parts[i+1], MAP) {
						mapOperator = string(parts[i+2])
						opMAP = i + 3
						break parseLoop
					}
				}
			}
		}
		if opORD == 0 && bytes.Equal(op, []byte("ord")) && opIf == i-1 && opFalse == i-2 {
			opORD = i
			endLock = i - 2
			lockScript = lockScript[:len(lockScript)-2]
		}
		if endLock > 0 {
			continue
		}
		if len(op) == 1 {
			lockScript.AppendOpcodes(opcode)
		} else {
			lockScript.AppendPushData(op)
		}
	}

	var hash [32]byte
	if endLock == 0 {
		hash = sha256.Sum256(script)
	} else {
		hash = sha256.Sum256(lockScript)
	}
	p = &ParsedScript{
		Lock: bt.ReverseBytes(hash[:]),
	}
	if opORD > 0 {
		p.Inscription = &Inscription{}
		var pos int
	ordLoop:
		for pos = opORD + 1; pos < len(parts); pos += 2 {
			op := parts[pos]
			if len(op) != 1 {
				break
			}
			opcode := op[0]
			switch opcode {
			case bscript.Op0:
				value := parts[pos+1]
				if len(value) == 1 && value[0] == bscript.Op0 {
					value = []byte{}
				}
				p.Inscription.Body = value
				break ordLoop
			case bscript.Op1:
				value := parts[pos+1]
				if len(value) == 1 && value[0] == bscript.Op0 {
					value = []byte{}
				}
				p.Inscription.Type = string(value)
			case bscript.OpENDIF:
				break ordLoop
			}
		}
		if includeFileMeta {
			hash := sha256.Sum256(p.Inscription.Body)
			p.File = File{
				Hash: hash[:],
				Size: uint32(len(p.Inscription.Body)),
				Type: p.Inscription.Type,
			}

		}
	}
	if opMAP > 0 && mapOperator == "SET" {
		p.Map = map[string]string{}
		for pos := opMAP; pos < len(parts); pos += 2 {
			op := parts[pos]
			if len(op) == 1 {
				opcode := op[0]
				if opcode == bscript.OpSWAP {
					break
				}
			}
			if len(parts) > pos+1 {
				p.Map[string(op)] = string(parts[pos+1])
			}
		}
	}

	return
}

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

	rows, err = GetUnnumbered.Query(height)
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
