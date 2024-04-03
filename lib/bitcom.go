package lib

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"strconv"
	"unicode/utf8"

	"github.com/bitcoinschema/go-bitcoin"
	"github.com/libsv/go-bt/v2"
	"github.com/libsv/go-bt/v2/bscript"
)

var MAP = "1PuQa7K62MiKCtssSLKy1kh56WWU7MtUR5"
var B = "19HxigV4QyBv3tHpQVcUEQyq1pzZVdoAut"

func ParseBitcom(tx *bt.Tx, vout uint32, idx *int) (value interface{}, err error) {
	script := *tx.Outputs[vout].LockingScript

	startIdx := *idx
	op, err := ReadOp(script, idx)
	if err != nil {
		return
	}
	switch string(op.Data) {
	case MAP:
		value = ParseMAP(&script, idx)

	case B:
		value = ParseB(&script, idx)
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
	return value, nil
}

func ParseMAP(script *bscript.Script, idx *int) (mp Map) {
	op, err := ReadOp(*script, idx)
	if err != nil {
		return
	}
	if string(op.Data) != "SET" {
		return nil
	}
	mp = Map{}
	for {
		prevIdx := *idx
		op, err = ReadOp(*script, idx)
		if err != nil || op.OpCode == bscript.OpRETURN || (op.OpCode == 1 && op.Data[0] == '|') {
			*idx = prevIdx
			break
		}
		opKey := op.Data
		prevIdx = *idx
		op, err = ReadOp(*script, idx)
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

		mp[string(opKey)] = string(op.Data)

	}
	if val, ok := mp["subTypeData"].(string); ok {
		if bytes.Contains([]byte(val), []byte{0}) || bytes.Contains([]byte(val), []byte("\\u0000")) {
			delete(mp, "subTypeData")
		} else {
			var subTypeData json.RawMessage
			if err := json.Unmarshal([]byte(val), &subTypeData); err == nil {
				mp["subTypeData"] = subTypeData
			}
		}
	}

	return
}

func ParseB(script *bscript.Script, idx *int) (b *File) {
	b = &File{}
	for i := 0; i < 4; i++ {
		prevIdx := *idx
		op, err := ReadOp(*script, idx)
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
	return
}
