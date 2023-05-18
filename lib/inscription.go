package lib

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"database/sql/driver"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"

	"github.com/bitcoinschema/go-bitcoin"
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
	Valid     bool   `json:"valid"`
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

type OpPart struct {
	OpCode byte
	Data   []byte
	Len    uint32
}

func ReadOp(b []byte, idx *int) (op *OpPart, err error) {
	if len(b) <= *idx {
		// log.Panicf("ReadOp: %d %d", len(b), *idx)
		err = fmt.Errorf("ReadOp: %d %d", len(b), *idx)
		return
	}
	switch b[*idx] {
	case bscript.OpPUSHDATA1:
		if len(b) < *idx+2 {
			err = bscript.ErrDataTooSmall
			return
		}

		l := int(b[*idx+1])
		*idx += 2

		if len(b) < *idx+l {
			err = bscript.ErrDataTooSmall
			return
		}

		op = &OpPart{OpCode: bscript.OpPUSHDATA1, Data: b[*idx : *idx+l]}
		*idx += l

	case bscript.OpPUSHDATA2:
		if len(b) < *idx+3 {
			err = bscript.ErrDataTooSmall
			return
		}

		l := int(binary.LittleEndian.Uint16(b[*idx+1:]))
		*idx += 3

		if len(b) < *idx+l {
			err = bscript.ErrDataTooSmall
			return
		}

		op = &OpPart{OpCode: bscript.OpPUSHDATA2, Data: b[*idx : *idx+l]}
		*idx += l

	case bscript.OpPUSHDATA4:
		if len(b) < *idx+5 {
			err = bscript.ErrDataTooSmall
			return
		}

		l := int(binary.LittleEndian.Uint32(b[*idx+1:]))
		*idx += 5

		if len(b) < *idx+l {
			err = bscript.ErrDataTooSmall
			return
		}

		op = &OpPart{OpCode: bscript.OpPUSHDATA4, Data: b[*idx : *idx+l]}
		*idx += l

	default:
		if b[*idx] >= 0x01 && b[*idx] < bscript.OpPUSHDATA1 {
			l := b[*idx]
			if len(b) < *idx+int(1+l) {
				err = bscript.ErrDataTooSmall
				return
			}
			op = &OpPart{OpCode: b[*idx], Data: b[*idx+1 : *idx+int(l+1)]}
			*idx += int(1 + l)
		} else {
			op = &OpPart{OpCode: b[*idx]}
			*idx++
		}
	}

	return
}

func ParseBitcom(script []byte, idx *int, p *ParsedScript, tx *bt.Tx) (err error) {
	startIdx := *idx
	op, err := ReadOp(script, idx)
	if err != nil {
		return
	}
	switch string(op.Data) {
	case MAP:
		op, err = ReadOp(script, idx)
		if err != nil {
			return
		}
		if string(op.Data) != "SET" {
			return nil
		}
		p.Map = map[string]interface{}{}
		for {
			prevIdx := *idx
			op, err = ReadOp(script, idx)
			if err != nil || op.OpCode == bscript.OpRETURN || (op.OpCode == 1 && op.Data[0] == '|') {
				*idx = prevIdx
				break
			}
			opKey := string(op.Data)
			prevIdx = *idx
			op, err = ReadOp(script, idx)
			if err != nil || op.OpCode == bscript.OpRETURN || (op.OpCode == 1 && op.Data[0] == '|') {
				*idx = prevIdx
				break
			}
			// fmt.Println(opKey, op.OpCode, string(op.Data))
			p.Map[opKey] = string(op.Data)
		}
		if val, ok := p.Map["subTypeData"]; ok {
			var subTypeData json.RawMessage
			if err := json.Unmarshal([]byte(val.(string)), &subTypeData); err == nil {
				p.Map["subTypeData"] = subTypeData
			}
		}
		return nil
	case B:
		p.B = &File{}
		for i := 0; i < 4; i++ {
			prevIdx := *idx
			op, err = ReadOp(script, idx)
			if err != nil || op.OpCode == bscript.OpRETURN || (op.OpCode == 1 && op.Data[0] == '|') {
				*idx = prevIdx
				break
			}

			switch i {
			case 0:
				p.B.Content = op.Data
			case 1:
				p.B.Type = string(op.Data)
			case 2:
				p.B.Encoding = string(op.Data)
			case 3:
				p.B.Name = string(op.Data)
			}
		}
		hash := sha256.Sum256(p.B.Content)
		p.B.Size = uint32(len(p.B.Content))
		p.B.Hash = hash[:]
	case "SIGMA":
		sigma := &Sigma{}
		for i := 0; i < 4; i++ {
			prevIdx := *idx
			op, err = ReadOp(script, idx)
			if err != nil || op.OpCode == bscript.OpRETURN || (op.OpCode == 1 && op.Data[0] == '|') {
				*idx = prevIdx
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
				vin, err := strconv.ParseInt(string(op.Data), 10, 32)
				if err == nil {
					sigma.Vin = uint32(vin)
				}
			}
		}
		p.Sigmas = append(p.Sigmas, sigma)

		outpoint := tx.Inputs[sigma.Vin].PreviousTxID()
		outpoint = binary.LittleEndian.AppendUint32(outpoint, tx.Inputs[sigma.Vin].PreviousTxOutIndex)
		// fmt.Printf("outpoint %x\n", outpoint)
		inputHash := sha256.Sum256(outpoint)
		// fmt.Printf("ihash: %x\n", inputHash)
		var scriptBuf []byte
		if script[startIdx-1] == bscript.OpRETURN {
			scriptBuf = script[:startIdx-1]
		} else if script[startIdx-1] == '|' {
			scriptBuf = script[:startIdx-2]
		} else {
			return nil
		}
		// fmt.Printf("scriptBuf %x\n", scriptBuf)
		outputHash := sha256.Sum256(scriptBuf)
		// fmt.Printf("ohash: %x\n", outputHash)
		msgHash := sha256.Sum256(append(inputHash[:], outputHash[:]...))
		// fmt.Printf("msghash: %x\n", msgHash)
		err = bitcoin.VerifyMessage(sigma.Address,
			base64.StdEncoding.EncodeToString(sigma.Signature),
			string(msgHash[:]),
		)
		if err != nil {
			fmt.Println("Error verifying signature", err)
			return nil
		}
		sigma.Valid = true
	default:
		*idx--
	}
	return
}

func ParseScript(script bscript.Script, tx *bt.Tx) (p *ParsedScript) {
	p = &ParsedScript{
		Sigmas: make(Sigmas, 0),
	}

	var opFalse int
	var opIf int
	var endLock int
	for i := 0; i < len(script); {
		startIdx := i
		op, err := ReadOp(script, &i)
		if err != nil {
			break
		}
		// fmt.Println(prevI, i, op)
		switch op.OpCode {
		case bscript.Op0:
			opFalse = startIdx
		case bscript.OpIF:
			opIf = startIdx
		case bscript.OpRETURN:
			// fmt.Println("RETURN", startIdx, op.Data)
			if endLock == 0 {
				endLock = startIdx
			}
			if endLock > 0 {
				err = ParseBitcom(script, &i, p, tx)
				if err != nil {
					log.Println("Error parsing bitcom", err)
					continue
				}
			}
		case bscript.OpDATA1:
			// fmt.Println("DATA1", startIdx, op.Data)
			if op.Data[0] == '|' && endLock > 0 {
				err = ParseBitcom(script, &i, p, tx)
				if err != nil {
					log.Println("Error parsing bitcom", err)
					continue
				}
			}
		}

		if bytes.Equal(op.Data, []byte("ord")) && opIf == startIdx-1 && opFalse == startIdx-2 {
			if endLock == 0 {
				endLock = startIdx - 2
				// lockParts = lockParts[:len(lockParts)-2]
				// lockScript = lockScript[:len(lockScript)-2]
			}
			p.Ord = &File{}
		ordLoop:
			for {
				op, err = ReadOp(script, &i)
				if err != nil {
					break
				}
				switch op.OpCode {
				case bscript.Op0:
					op, err = ReadOp(script, &i)
					if err != nil {
						break ordLoop
					}
					p.Ord.Content = op.Data
				case bscript.Op1:
					op, err = ReadOp(script, &i)
					if err != nil {
						break ordLoop
					}
					p.Ord.Type = string(op.Data)
				case bscript.OpENDIF:
					break ordLoop
				}
			}
			hash := sha256.Sum256(p.Ord.Content)
			p.Ord.Size = uint32(len(p.Ord.Content))
			p.Ord.Hash = hash[:]
		}
	}

	var lockScript bscript.Script

	// fmt.Println("endLock", endLock)
	if endLock > 0 {
		lockScript = script[:endLock]
	} else {
		lockScript = script
	}
	// asm, err := lockScript.ToASM()
	// if err != nil {
	// 	fmt.Println("Error converting to asm", err)
	// }
	// fmt.Printf("lockScript: %s\n", asm)

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
