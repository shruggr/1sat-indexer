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
	"strconv"
	"strings"

	"github.com/libsv/go-bt/v2"
	"github.com/libsv/go-bt/v2/bscript"
)

var PATTERN []byte
var MAP = "1PuQa7K62MiKCtssSLKy1kh56WWU7MtUR5"
var B = "19HxigV4QyBv3tHpQVcUEQyq1pzZVdoAut"

type Map map[string]interface{}

func (m Map) Value() (driver.Value, error) {
	if m == nil {
		return nil, nil
	}
	return json.Marshal(m)
}

func (m *Map) Scan(value interface{}) error {
	b, ok := value.([]byte)
	if !ok {
		return errors.New("type assertion to []byte failed")
	}
	return json.Unmarshal(b, &m)
}

var OrdLockPrefix []byte
var OrdLockSuffix []byte

func init() {
	val, err := hex.DecodeString("0063036f7264")
	if err != nil {
		log.Panic(err)
	}
	PATTERN = val

	OrdLockPrefix, _ = hex.DecodeString("2097dfd76851bf465e8f715593b217714858bbe9570ff3bd5e33840a34e20ff0262102ba79df5f8ae7604a9830f03c7933028186aede0675a16f025dc4f8be8eec0382201008ce7480da41702918d1ec8e6849ba32b4d65b1e40dc669c31a1e6306b266c0000")
	OrdLockSuffix, _ = hex.DecodeString("615179547a75537a537a537a0079537a75527a527a7575615579008763567901c161517957795779210ac407f0e4bd44bfc207355a778b046225a7068fc59ee7eda43ad905aadbffc800206c266b30e6a1319c66dc401e5bd6b432ba49688eecd118297041da8074ce081059795679615679aa0079610079517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e01007e81517a75615779567956795679567961537956795479577995939521414136d08c5ed2bf3ba048afe6dcaebafeffffffffffffffffffffffffffffff00517951796151795179970079009f63007952799367007968517a75517a75517a7561527a75517a517951795296a0630079527994527a75517a6853798277527982775379012080517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e01205279947f7754537993527993013051797e527e54797e58797e527e53797e52797e57797e0079517a75517a75517a75517a75517a75517a75517a75517a75517a75517a75517a75517a75517a756100795779ac517a75517a75517a75517a75517a75517a75517a75517a75517a7561517a75517a756169587951797e58797eaa577961007982775179517958947f7551790128947f77517a75517a75618777777777777777777767557951876351795779a9876957795779ac777777777777777767006868")
}

// type Inscription struct {
// 	Content []byte
// 	Type    string
// }

type File struct {
	Hash     ByteString `json:"hash"`
	Size     uint32     `json:"size"`
	Type     string     `json:"type"`
	Content  []byte     `json:"-"`
	Encoding string     `json:"encoding,omitempty"`
	Name     string     `json:"name,omitempty"`
}

func (f File) Value() (driver.Value, error) {
	return json.Marshal(f)
}

func (f *File) Scan(value interface{}) error {
	b, ok := value.([]byte)
	if !ok {
		return errors.New("type assertion to []byte failed")
	}
	return json.Unmarshal(b, &f)
}

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
}

type ParsedScript struct {
	Id       uint64            `json:"id"`
	Txid     ByteString        `json:"txid"`
	Vout     uint32            `json:"vout"`
	Ord      *File             `json:"file"`
	Origin   *Outpoint         `json:"origin"`
	Ordinal  uint32            `json:"ordinal"`
	Height   uint32            `json:"height"`
	Idx      uint32            `json:"idx"`
	Lock     ByteString        `json:"lock"`
	Map      Map               `json:"MAP,omitempty"`
	B        *File             `json:"B,omitempty"`
	Listings []*OrdLockListing `json:"listings,omitempty"`
	Sigmas   Sigmas            `json:"sigma,omitempty"`
	// Inscription *Inscription `json:"-"`
}

func (p *ParsedScript) SaveInscription() (err error) {
	_, err = InsInscription.Exec(
		p.Txid,
		p.Vout,
		p.Height,
		p.Idx,
		p.Ord.Hash,
		p.Ord.Size,
		p.Ord.Type,
		p.Map,
		p.Origin,
		p.Lock,
		p.Sigmas,
	)
	if err != nil {
		log.Panicf("Save Error: %x %d %x %+v\n", p.Txid, p.Ord.Size, p.Ord.Type, err)
	}
	return
}

func (p *ParsedScript) Save() (err error) {
	if p.Ord != nil || p.Map != nil || p.B != nil {
		_, err = InsMetadata.Exec(
			p.Txid,
			p.Vout,
			p.Height,
			p.Idx,
			p.Ord,
			p.Map,
			p.B,
			p.Origin,
			p.Sigmas,
		)
		if err != nil {
			log.Panicf("Save Error: %x %d %x %+v\n", p.Txid, p.Ord.Size, p.Ord.Type, err)
			log.Panic(err)
		}
	}
	return
}

func ParseScript(script bscript.Script, includeFileMeta bool) (p *ParsedScript) {
	p = &ParsedScript{
		Sigmas: make(Sigmas, 0),
	}
	parts, err := bscript.DecodeParts(script)
	if err != nil {
		hash := sha256.Sum256(script)
		p.Lock = bt.ReverseBytes(hash[:])
		// log.Panicf("Parsing Error: %x %+v\n", script, err)
		return
	}

	var opFalse int
	var opIf int
	var opORD int
	var opMAP int
	var opB int
	var opSIGMAs []int
	var endLock int
	var mapOperator string
	lockScript := bscript.Script{}

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
				if endLock > 0 {
					switch ParseBitcom(parts[i:]) {
					case MAP:
						mapOperator = string(parts[i+2])
						opMAP = i + 3
					case B:
						opB = i + 1
					case "SIGMA":
						opSIGMAs = append(opSIGMAs, i+1)
					}

				}
			case bscript.OpSWAP:
				if endLock > 0 {
					switch ParseBitcom(parts[i:]) {
					case MAP:
						mapOperator = string(parts[i+2])
						opMAP = i + 3
					case B:
						opB = i + 1
					case "SIGMA":
						opSIGMAs = append(opSIGMAs, i+1)
					}
				}
			}
		}

		if opORD == 0 && bytes.Equal(op, []byte("ord")) && opIf == i-1 && opFalse == i-2 {
			opORD = i
			if endLock == 0 {
				endLock = i - 2
				lockScript = lockScript[:len(lockScript)-2]
			}
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

	ordLockPrefixIndex := bytes.Index(script, OrdLockPrefix)
	ordLockSuffixIndex := bytes.Index(script, OrdLockSuffix)
	if ordLockPrefixIndex > -1 && ordLockSuffixIndex > len(OrdLockPrefix) {
		ordLock := script[ordLockPrefixIndex+len(OrdLockPrefix) : ordLockSuffixIndex]
		if ordLockParts, err := bscript.DecodeParts(ordLock); err == nil {
			pkh := ordLockParts[0]
			payOutput := &bt.Output{}
			_, err = payOutput.ReadFrom(bytes.NewReader(ordLockParts[1]))
			if err == nil {
				if owner, err := bscript.NewP2PKHFromPubKeyHash(pkh); err == nil {
					lockScript = *owner
					p.Listings = append(p.Listings, &OrdLockListing{
						Price:     payOutput.Satoshis,
						PayOutput: payOutput.Bytes(),
					})
				}
			}
		}
	}

	hash := sha256.Sum256(lockScript)
	p.Lock = bt.ReverseBytes(hash[:])
	if opORD > 0 {
		p.Ord = &File{}
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
				p.Ord.Content = value
			case bscript.Op1:
				value := parts[pos+1]
				if len(value) == 1 && value[0] == bscript.Op0 {
					value = []byte{}
				}
				p.Ord.Type = string(value)
			case bscript.OpENDIF:
				break ordLoop
			}
		}
		if includeFileMeta {
			hash := sha256.Sum256(p.Ord.Content)
			p.Ord.Size = uint32(len(p.Ord.Content))
			p.Ord.Hash = hash[:]
		}
	}
	if opMAP > 0 && mapOperator == "SET" {
		p.Map = map[string]interface{}{}
		for pos := opMAP; pos < len(parts); pos += 2 {
			op := parts[pos]
			if len(op) == 1 {
				opcode := op[0]
				if opcode == bscript.OpSWAP {
					break
				}
			}
			if len(parts) > pos+1 {
				p.Map[string(op)] = strings.ToValidUTF8(string(parts[pos+1]), "")
			}
		}
		if val, ok := p.Map["subTypeData"]; ok {
			var subTypeData json.RawMessage
			if err := json.Unmarshal([]byte(val.(string)), &subTypeData); err == nil {
				p.Map["subTypeData"] = subTypeData
			}
		}
	}

	if opB > 0 {
		p.B = &File{}
		for pos := opB; pos < opB+5; pos++ {
			op := parts[pos]
			var opcode byte
			if len(op) == 1 {
				opcode = op[0]
				if opcode == bscript.OpSWAP || opcode == bscript.OpRETURN {
					break
				}
				if opcode == bscript.Op0 {
					op = []byte{}
				}
			}

			switch pos {
			case opB + 1:
				p.B.Content = op
			case opB + 2:
				p.B.Type = string(op)
			case opB + 3:
				p.B.Encoding = string(op)
			case opB + 4:
				p.B.Name = string(op)
			}
		}
		if includeFileMeta {
			hash := sha256.Sum256(p.B.Content)
			p.B.Size = uint32(len(p.B.Content))
			p.B.Hash = hash[:]
		}
	}

	for _, opSIGMA := range opSIGMAs {
		if len(parts) < opSIGMA+4 {
			continue
		}
		sigma := &Sigma{
			Algorithm: string(parts[opSIGMA+1]),
			Address:   string(parts[opSIGMA+2]),
			Signature: parts[opSIGMA+3],
		}
		vin, err := strconv.ParseUint(string(parts[opSIGMA+4]), 10, 32)
		if err == nil {
			continue
		}
		sigma.Vin = uint32(vin)
		p.Sigmas = append(p.Sigmas, sigma)
	}

	return
}

func ParseBitcom(parts [][]byte) (bitcom string) {
	if len(parts) < 2 {
		return
	}
	return string(parts[1])
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
