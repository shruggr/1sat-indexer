package lock

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"log"

	"github.com/shruggr/1sat-indexer/v5/evt"
	"github.com/shruggr/1sat-indexer/v5/idx"
	"github.com/shruggr/1sat-indexer/v5/lib"
)

const LOCK_TAG = "lock"

type Lock struct {
	Address string `json:"address"`
	Until   uint32 `json:"until"`
}

type LockIndexer struct {
	idx.BaseIndexer
}

func (i *LockIndexer) Tag() string {
	return LOCK_TAG
}

func (i *LockIndexer) FromBytes(data []byte) (any, error) {
	obj := Lock{}
	if err := json.Unmarshal(data, &obj); err != nil {
		return nil, err
	}
	return obj, nil
}

func (i *LockIndexer) Parse(idxCtx *idx.IndexContext, vout uint32) *idx.IndexData {
	txo := idxCtx.Txos[vout]
	scr := idxCtx.Tx.Outputs[txo.Outpoint.Vout()].LockingScript
	lockPrefixIndex := bytes.Index(*scr, LockPrefix)
	if lockPrefixIndex > -1 && bytes.Contains((*scr)[lockPrefixIndex:], LockSuffix) {
		lock := &Lock{}
		pos := lockPrefixIndex + len(LockPrefix)
		if op, err := scr.ReadOp(&pos); err != nil {
			log.Println(err)
		} else if len(op.Data) != 20 {
			return nil
		} else {
			pkhash := lib.PKHash(op.Data)
			lock.Address = pkhash.Address(idxCtx.Network)
			txo.AddOwner(lock.Address)
		}
		if op, err := scr.ReadOp(&pos); err != nil {
			log.Println(err)
		} else {
			until := make([]byte, 4)
			copy(until, op.Data)
			lock.Until = binary.LittleEndian.Uint32(until)
			return &idx.IndexData{
				Data: lock,
				Events: []*evt.Event{
					{Id: "own", Value: lock.Address},
				},
			}
		}
	}
	return nil
}

var LockPrefix, _ = hex.DecodeString("2097dfd76851bf465e8f715593b217714858bbe9570ff3bd5e33840a34e20ff0262102ba79df5f8ae7604a9830f03c7933028186aede0675a16f025dc4f8be8eec0382201008ce7480da41702918d1ec8e6849ba32b4d65b1e40dc669c31a1e6306b266c0000")
var LockSuffix, _ = hex.DecodeString("610079040065cd1d9f690079547a75537a537a537a5179537a75527a527a7575615579014161517957795779210ac407f0e4bd44bfc207355a778b046225a7068fc59ee7eda43ad905aadbffc800206c266b30e6a1319c66dc401e5bd6b432ba49688eecd118297041da8074ce081059795679615679aa0079610079517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e01007e81517a75615779567956795679567961537956795479577995939521414136d08c5ed2bf3ba048afe6dcaebafeffffffffffffffffffffffffffffff00517951796151795179970079009f63007952799367007968517a75517a75517a7561527a75517a517951795296a0630079527994527a75517a6853798277527982775379012080517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e01205279947f7754537993527993013051797e527e54797e58797e527e53797e52797e57797e0079517a75517a75517a75517a75517a75517a75517a75517a75517a75517a75517a75517a75517a756100795779ac517a75517a75517a75517a75517a75517a75517a75517a75517a7561517a75517a756169557961007961007982775179517954947f75517958947f77517a75517a756161007901007e81517a7561517a7561040065cd1d9f6955796100796100798277517951790128947f755179012c947f77517a75517a756161007901007e81517a7561517a756105ffffffff009f69557961007961007982775179517954947f75517958947f77517a75517a756161007901007e81517a7561517a75615279a2695679a95179876957795779ac7777777777777777")
