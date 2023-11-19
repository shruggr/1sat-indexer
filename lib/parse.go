package lib

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"unicode/utf8"

	"github.com/bitcoinschema/go-bitcoin"
	"github.com/libsv/go-bt/v2"
	"github.com/libsv/go-bt/v2/bscript"
)

var MAP = "1PuQa7K62MiKCtssSLKy1kh56WWU7MtUR5"
var B = "19HxigV4QyBv3tHpQVcUEQyq1pzZVdoAut"
var SCryptPrefix, _ = hex.DecodeString("2097dfd76851bf465e8f715593b217714858bbe9570ff3bd5e33840a34e20ff0262102ba79df5f8ae7604a9830f03c7933028186aede0675a16f025dc4f8be8eec0382201008ce7480da41702918d1ec8e6849ba32b4d65b1e40dc669c31a1e6306b266c0000")

type OpPart struct {
	OpCode byte
	Data   []byte
	Len    uint32
}

var AsciiRegexp = regexp.MustCompile(`^[[:ascii:]]*$`)

func ReadOp(b []byte, idx *int) (op *OpPart, err error) {
	if len(b) <= *idx {
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

	op.Len = uint32(len(op.Data))
	return
}

func ParseBitcom(tx *bt.Tx, vout uint32, idx *int) (value interface{}, err error) {
	script := *tx.Outputs[vout].LockingScript

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
			return nil, nil
		}
		Map := map[string]interface{}{}
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

			if len(opKey) > 256 || len(op.Data) > 1024 {
				continue
			}

			if !utf8.Valid(opKey) || !utf8.Valid(op.Data) {
				continue
			}

			if len(opKey) == 1 && opKey[0] == 0 {
				opKey = []byte{}
			}
			if len(op.Data) == 1 && op.Data[0] == 0 {
				op.Data = []byte{}
			}

			Map[string(opKey)] = string(op.Data)

		}
		if val, ok := Map["subTypeData"]; ok {
			var subTypeData json.RawMessage
			if err := json.Unmarshal([]byte(val.(string)), &subTypeData); err == nil {
				Map["subTypeData"] = subTypeData
			}
		}
		value = Map

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
		value = b
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

		outpoint := tx.Inputs[sigma.Vin].PreviousTxID()
		outpoint = binary.LittleEndian.AppendUint32(outpoint, tx.Inputs[sigma.Vin].PreviousTxOutIndex)
		inputHash := sha256.Sum256(outpoint)
		var scriptBuf []byte
		if script[startIdx-1] == bscript.OpRETURN {
			scriptBuf = script[:startIdx-1]
		} else if script[startIdx-1] == '|' {
			scriptBuf = script[:startIdx-2]
		} else {
			return nil, nil
		}
		outputHash := sha256.Sum256(scriptBuf)
		msgHash := sha256.Sum256(append(inputHash[:], outputHash[:]...))
		err = bitcoin.VerifyMessage(sigma.Address,
			base64.StdEncoding.EncodeToString(sigma.Signature),
			string(msgHash[:]),
		)
		if err != nil {
			return nil, nil
		}
		sigma.Valid = true

		value = sigma
	default:
		*idx--
	}
	return
}
