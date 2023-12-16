package opns

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"log"

	"github.com/libsv/go-bt"
	"github.com/shruggr/1sat-indexer/lib"
	"github.com/shruggr/1sat-indexer/ordinals"
)

// type Claim struct {
// 	SubDomain string `json:"sub"`
// 	Type      string `json:"type"`
// 	Value     string `json:"value"`
// }

type OpNS struct {
	Genesis *lib.Outpoint `json:"genesis,omitempty"`
	Domain  string        `json:"domain"`
	Status  int           `json:"status"`
	PoW     []byte        `json:"pow,omitempty"`
}

var GENESIS, _ = lib.NewOutpointFromString("58b7558ea379f24266c7e2f5fe321992ad9a724fd7a87423ba412677179ccb25_0")

func IndexTxn(rawtx []byte, blockId string, height uint32, idx uint64, dryRun bool) (ctx *lib.IndexContext) {
	ctx, err := lib.IndexTxn(rawtx, blockId, height, idx, dryRun)
	if err != nil {
		panic(err)
	}
	IndexOpNS(ctx)
	return
}

func IndexOpNS(ctx *lib.IndexContext) {
	for _, txo := range ctx.Txos {
		if len(txo.PKHash) != 0 {
			continue
		}
		opNsMine := ParseScript(txo)
		if opNsMine != nil {
			srcMine := loadSrcMine(ctx)
			if srcMine == nil && bytes.Equal(*txo.Outpoint, *GENESIS) {
				opNsMine.Genesis = txo.Outpoint
				opNsMine.Status = 1
			} else if srcMine != nil && srcMine.Status == 1 && bytes.Equal(*opNsMine.Genesis, *GENESIS) {
				opNsMine.Status = 1
			} else {
				opNsMine.Status = -1
			}
			txo.AddData("opnsMine", opNsMine)
			txo.Save()
			continue
		}
		ordinals.ParseScript(txo)
		if txo.Data == nil {
			continue
		}
		insc, ok := txo.Data["insc"].(*ordinals.Inscription)
		if !ok || insc.File.Type != "application/op-ns" {
			continue
		}

		srcMine := loadSrcMine(ctx)
		if srcMine != nil {
			txo.AddData("opns", &OpNS{
				Genesis: srcMine.Genesis,
				Domain:  insc.Text,
				Status:  srcMine.Status,
			})
			txo.Save()
		}
	}
}

func loadSrcMine(ctx *lib.IndexContext) (srcMine *OpNS) {
	source := ctx.Spends[0].Outpoint
	srcData, _ := lib.LoadTxoData(source)
	for i := 0; i < 2; i++ {
		if srcData != nil {
			if mineData, ok := srcData["opnsMine"]; ok {
				j, err := json.Marshal(mineData)
				if err == nil {
					srcMine = &OpNS{}
					json.Unmarshal(j, srcMine)
				}
				return srcMine
			}
		}
		if i > 0 {
			break
		}
		srcTx, err := lib.LoadRawtx(hex.EncodeToString(ctx.Spends[0].Outpoint.Txid()))
		if err != nil {
			log.Panicln(err)
		}
		srcCtx := IndexTxn(srcTx, "", 0, 0, true)
		srcData = srcCtx.Txos[source.Vout()].Data
	}
	return srcMine
}

func ParseScript(txo *lib.Txo) (opNS *OpNS) {
	script := *txo.Tx.Outputs[txo.Outpoint.Vout()].LockingScript
	opNSPrefixIndex := bytes.Index(script, OpNSPrefix)
	if opNSPrefixIndex > -1 {
		opNSSuffixIndex := bytes.Index(script, OpNSSuffix)
		if opNSSuffixIndex > -1 {
			opNS = &OpNS{}
			stateScript := script[opNSSuffixIndex+len(OpNSSuffix)+2:]
			pos := 0
			op, err := lib.ReadOp(stateScript, &pos)
			if err != nil {
				return
			}
			if op.Len == 36 {
				genesis := op.Data
				txid := bt.ReverseBytes(genesis[:32])
				vout := binary.LittleEndian.Uint32(genesis[32:36])
				opNS.Genesis = lib.NewOutpoint(txid, vout)
			}
			if _, err = lib.ReadOp(stateScript, &pos); err != nil {
				return
			}
			if op, err = lib.ReadOp(stateScript, &pos); err != nil {
				return
			}
			opNS.Domain = string(op.Data)
			if op, err = lib.ReadOp(stateScript, &pos); err != nil {
				return
			}
			opNS.PoW = op.Data
		}
	}
	return
}