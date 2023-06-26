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
	"strings"
	"unicode/utf8"

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
	val, err := json.Marshal(m)
	// log.Printf("%s\n", val)
	return val, err
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

type Inscription struct {
	Num         uint64          `json:"num"`
	Txid        ByteString      `json:"txid"`
	Vout        uint32          `json:"vout"`
	JsonContent json.RawMessage `json:"content"`
	File        *File           `json:"file,omitempty"`
}

type Sigma struct {
	Algorithm string `json:"algorithm"`
	Address   string `json:"address"`
	Signature []byte `json:"signature"`
	Vin       uint32 `json:"vin"`
	Valid     bool   `json:"valid"`
}

type ParsedScript struct {
	Num         uint64          `json:"num"`
	Txid        ByteString      `json:"txid"`
	Vout        uint32          `json:"vout"`
	Inscription *Inscription    `json:"inscription"`
	Origin      *Outpoint       `json:"origin"`
	Ordinal     uint32          `json:"ordinal"`
	Lock        ByteString      `json:"lock"`
	Map         Map             `json:"MAP,omitempty"`
	B           *File           `json:"B,omitempty"`
	Listing     *OrdLockListing `json:"listings,omitempty"`
	Sigmas      Sigmas          `json:"sigma,omitempty"`
	Bsv20       *Bsv20          `json:"bsv20,omitempty"`
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
			opKey := op.Data
			prevIdx = *idx
			op, err = ReadOp(script, idx)
			if err != nil || op.OpCode == bscript.OpRETURN || (op.OpCode == 1 && op.Data[0] == '|') {
				*idx = prevIdx
				break
			}

			if !utf8.Valid(opKey) || !utf8.Valid(op.Data) {
				continue
			}

			log.Println(opKey, op.Data)
			if len(opKey) == 1 && opKey[0] == 0 {
				opKey = []byte{}
			}
			if len(op.Data) == 1 && op.Data[0] == 0 {
				op.Data = []byte{}
			}
			p.Map[string(opKey)] = string(op.Data)
		}
		if val, ok := p.Map["subTypeData"]; ok {
			var subTypeData json.RawMessage
			if err := json.Unmarshal([]byte(val.(string)), &subTypeData); err == nil {
				p.Map["subTypeData"] = subTypeData
			}
		}
		return nil
	case B:
		b := &File{}
		for i := 0; i < 4; i++ {
			prevIdx := *idx
			op, err = ReadOp(script, idx)
			if err != nil || op.OpCode == bscript.OpRETURN || (op.OpCode == 1 && op.Data[0] == '|') {
				*idx = prevIdx
				break
			}

			switch i {
			case 0:
				b.Content = op.Data
			case 1:
				b.Type = string(op.Data)
			case 2:
				b.Encoding = string(op.Data)
			case 3:
				b.Name = string(op.Data)
			}
		}
		hash := sha256.Sum256(b.Content)
		b.Size = uint32(len(b.Content))
		b.Hash = hash[:]
		p.B = b
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

func ParseScript(script bscript.Script, tx *bt.Tx, height uint32) (p *ParsedScript) {
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
			inscription := &File{}
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
					inscription.Content = op.Data
				case bscript.Op1:
					op, err = ReadOp(script, &i)
					if err != nil {
						break ordLoop
					}
					inscription.Type = string(op.Data)
				case bscript.OpENDIF:
					break ordLoop
				}
			}
			hash := sha256.Sum256(inscription.Content)
			inscription.Size = uint32(len(inscription.Content))
			inscription.Hash = hash[:]
			p.Inscription = &Inscription{
				File: inscription,
			}

			if inscription.Size <= 1024 && utf8.Valid(inscription.Content) {
				mime := strings.ToLower(inscription.Type)
				if strings.HasPrefix(mime, "application/bsv-20") ||
					strings.HasPrefix(mime, "text/plain") ||
					strings.HasPrefix(mime, "application/json") {

					var data json.RawMessage
					err = json.Unmarshal(inscription.Content, &data)
					if err == nil {
						p.Inscription.JsonContent = data
						if strings.HasPrefix(mime, "application/bsv-20") ||
							(height >= 793000 && strings.HasPrefix(mime, "text/plain")) {

							p.Bsv20, _ = parseBsv20(inscription.Content)
						}
					}
				}
			}
		}
	}

	var lockScript bscript.Script
	// fmt.Println("endLock", endLock)

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
					p.Listing = &OrdLockListing{
						Price:     payOutput.Satoshis,
						PayOutput: payOutput.Bytes(),
					}
				}
			}
		}
	} else if endLock > 0 {
		lockScript = script[:endLock]
	}

	hash := sha256.Sum256(lockScript)
	p.Lock = bt.ReverseBytes(hash[:])

	return
}

func SetOriginNums(height uint32) (err error) {
	rows, err := Db.Query(`SELECT MAX(num) FROM origins`)
	if err != nil {
		log.Panic(err)
		return
	}
	defer rows.Close()
	var num uint64
	if rows.Next() {
		var dbId sql.NullInt64
		err = rows.Scan(&dbId)
		if err != nil {
			log.Panic(err)
		}
		if dbId.Valid {
			num = uint64(dbId.Int64 + 1)
		}
	}

	rows, err = Db.Query(`SELECT origin
		FROM origins
		WHERE num = -1 AND height <= $1 AND height > 0
		ORDER BY height, idx, vout`,
		height,
	)
	if err != nil {
		log.Panic(err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var origin []byte
		err = rows.Scan(&origin)
		if err != nil {
			log.Panic(err)
		}
		fmt.Printf("Origin Num %d %x\n", num, origin)
		_, err = Db.Exec(`UPDATE origins
			SET num=$2
			WHERE origin=$1`,
			origin,
			num,
		)
		if err != nil {
			log.Panic(err)
			return
		}
		num++
	}
	return
}
