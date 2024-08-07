package lock

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"log"

	"github.com/shruggr/1sat-indexer/lib"
)

type Lock struct {
	Address string `json:"address"`
	Until   uint32 `json:"until"`
}

var LockSuffix, _ = hex.DecodeString("610079040065cd1d9f690079547a75537a537a537a5179537a75527a527a7575615579014161517957795779210ac407f0e4bd44bfc207355a778b046225a7068fc59ee7eda43ad905aadbffc800206c266b30e6a1319c66dc401e5bd6b432ba49688eecd118297041da8074ce081059795679615679aa0079610079517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e01007e81517a75615779567956795679567961537956795479577995939521414136d08c5ed2bf3ba048afe6dcaebafeffffffffffffffffffffffffffffff00517951796151795179970079009f63007952799367007968517a75517a75517a7561527a75517a517951795296a0630079527994527a75517a6853798277527982775379012080517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e01205279947f7754537993527993013051797e527e54797e58797e527e53797e52797e57797e0079517a75517a75517a75517a75517a75517a75517a75517a75517a75517a75517a75517a75517a756100795779ac517a75517a75517a75517a75517a75517a75517a75517a75517a7561517a75517a756169557961007961007982775179517954947f75517958947f77517a75517a756161007901007e81517a7561517a7561040065cd1d9f6955796100796100798277517951790128947f755179012c947f77517a75517a756161007901007e81517a7561517a756105ffffffff009f69557961007961007982775179517954947f75517958947f77517a75517a756161007901007e81517a7561517a75615279a2695679a95179876957795779ac7777777777777777")

func IndexTxn(rawtx []byte, blockId string, height uint32, idx uint64, dryRun bool) (ctx *lib.IndexContext) {
	ctx, err := lib.ParseTxn(rawtx, blockId, height, idx)
	if err != nil {
		log.Panicln(err)
	}
	IndexLocks(ctx)
	return
}

func IndexLocks(ctx *lib.IndexContext) {
	ParseLocks(ctx)
	for _, txo := range ctx.Txos {
		txo.Save()
	}
}

func ParseLocks(ctx *lib.IndexContext) {
	for _, txo := range ctx.Txos {
		if txo.PKHash != nil && len(*txo.PKHash) > 0 {
			continue
		}
		lock := ParseScript(txo)
		if lock == nil {
			continue
		}
		txo.AddData("lock", lock)
	}
}

func ParseScript(txo *lib.Txo) (lock *Lock) {
	script := *txo.Tx.Outputs[txo.Outpoint.Vout()].LockingScript
	sCryptPrefixIndex := bytes.Index(script, lib.SCryptPrefix)
	if sCryptPrefixIndex > -1 && bytes.Contains(script[sCryptPrefixIndex:], LockSuffix) {
		lock = &Lock{}
		pos := sCryptPrefixIndex + len(lib.SCryptPrefix)
		op, err := lib.ReadOp(script, &pos)
		if err != nil {
			log.Println(err)
		}
		if len(op.Data) == 20 {
			pkhash := lib.PKHash(op.Data)
			txo.PKHash = &pkhash
			if address, err := pkhash.Address(); err == nil {
				lock.Address = address
			}
		}
		op, err = lib.ReadOp(script, &pos)
		if err != nil {
			log.Println(err)
		}
		until := make([]byte, 4)
		copy(until, op.Data)
		lock.Until = binary.LittleEndian.Uint32(until)
	}
	return
}
