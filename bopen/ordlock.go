package bopen

import (
	"bytes"
	"encoding/hex"
	"math"

	// "github.com/libsv/go-bt"
	// "github.com/libsv/go-bt/bscript"
	"github.com/bitcoin-sv/go-sdk/script"
	"github.com/bitcoin-sv/go-sdk/transaction"
	"github.com/shruggr/1sat-indexer/evt"
	"github.com/shruggr/1sat-indexer/idx"
	"github.com/shruggr/1sat-indexer/lib"
)

const ORDLOCK_TAG = "ordlock"

type OrdLockState int

var (
	OrdLockCancel  OrdLockState = -1
	OrdLockPending OrdLockState = 0
	OrdLockSale    OrdLockState = 1
)

type OrdLock struct {
	Price    uint64       `json:"price"`
	PricePer float64      `json:"pricePer"`
	PayOut   []byte       `json:"payout"`
	Tags     []string     `json:"tags,omitempty"`
	State    OrdLockState `json:"status"`
}

type OrdLockIndexer struct {
	idx.BaseIndexer
}

func (i *OrdLockIndexer) Tag() string {
	return ORDLOCK_TAG
}

func (i *OrdLockIndexer) Parse(idxCtx *idx.IndexContext, vout uint32) *idx.IndexData {
	txo := idxCtx.Txos[vout]
	if *txo.Satoshis != 1 {
		return nil
	}

	scr := idxCtx.Tx.Outputs[vout].LockingScript
	if sCryptPrefixIndex := bytes.Index(*scr, OrdLockPrefix); sCryptPrefixIndex == -1 {
		return nil
	} else if ordLockSuffixIndex := bytes.Index(*scr, OrdLockSuffix); ordLockSuffixIndex == -1 {
		return nil
	} else if ordLockOps, err := script.DecodeScript((*scr)[sCryptPrefixIndex+len(OrdLockPrefix) : ordLockSuffixIndex]); err != nil || len(ordLockOps) == 0 {
		return nil
	} else {
		pkhash := lib.PKHash(ordLockOps[0].Data)
		payOutput := &transaction.TransactionOutput{}
		if _, err = payOutput.ReadFrom(bytes.NewReader(ordLockOps[1].Data)); err != nil {
			return nil
		}
		txo.AddOwner(pkhash.Address())

		ordLock := &OrdLock{
			Price:  payOutput.Satoshis,
			PayOut: payOutput.Bytes(),
			State:  OrdLockPending,
			Tags:   make([]string, 0, 5),
		}
		idxData := &idx.IndexData{
			Data: ordLock,
		}
		ordLock.Tags = append(ordLock.Tags, "")
		ordLock.Tags = append(ordLock.Tags, pkhash.Address())
		if bsv21Data, ok := txo.Data[BSV21_TAG]; ok {
			bsv21 := bsv21Data.Data.(*Bsv21)
			ordLock.Tags = append(ordLock.Tags, bsv21.Id)
			ordLock.Tags = append(ordLock.Tags, BSV21_TAG)
			ordLock.PricePer = float64(ordLock.Price) / (float64(bsv21.Amt) / math.Pow(10, float64(bsv21.Decimals)))
		} else if bsv20Data, ok := txo.Data[BSV20_TAG]; ok {
			bsv20 := bsv20Data.Data.(*Bsv20)
			ordLock.Tags = append(ordLock.Tags, bsv20.Ticker)
			ordLock.Tags = append(ordLock.Tags, BSV20_TAG)
			ordLock.PricePer = float64(ordLock.Price) / (float64(*bsv20.Amt) / math.Pow(10, float64(bsv20.Decimals)))
		} else if idxData, ok := txo.Data[ORIGIN_TAG]; ok {
			origin := idxData.Data.(*Origin)
			if origin.Outpoint != nil {
				ordLock.Tags = append(ordLock.Tags, origin.Outpoint.String())
				ordLock.PricePer = float64(ordLock.Price)
			}
		}

		for _, tag := range ordLock.Tags {
			idxData.Events = append(idxData.Events, &evt.Event{
				Id:    "list",
				Value: tag,
			})
		}

		return idxData
	}
}

func (i *OrdLockIndexer) PreSave(idxCtx *idx.IndexContext) {
	if len(idxCtx.Spends) == 0 {
		return
	}
	spend := idxCtx.Spends[0]
	if spendData, ok := spend.Data[ORDLOCK_TAG]; ok {
		if ordLock, ok := spendData.Data.(*OrdLock); ok {
			txo := idxCtx.Txos[0]
			idxData := &idx.IndexData{
				Data: ordLock,
			}
			if bytes.Contains(*idxCtx.Tx.Inputs[0].UnlockingScript, OrdLockSuffix) {
				ordLock.State = OrdLockSale
				for _, tag := range ordLock.Tags {
					idxData.Events = append(idxData.Events, &evt.Event{
						Id:    "sale",
						Value: tag,
					})
				}
			} else {
				ordLock.State = OrdLockCancel
				for _, tag := range ordLock.Tags {
					idxData.Events = append(idxData.Events, &evt.Event{
						Id:    "cancel",
						Value: tag,
					})
				}
			}
			txo.Data[ORDLOCK_TAG] = idxData
		}
	}
}

var OrdLockPrefix, _ = hex.DecodeString("2097dfd76851bf465e8f715593b217714858bbe9570ff3bd5e33840a34e20ff0262102ba79df5f8ae7604a9830f03c7933028186aede0675a16f025dc4f8be8eec0382201008ce7480da41702918d1ec8e6849ba32b4d65b1e40dc669c31a1e6306b266c0000")
var OrdLockSuffix, _ = hex.DecodeString("615179547a75537a537a537a0079537a75527a527a7575615579008763567901c161517957795779210ac407f0e4bd44bfc207355a778b046225a7068fc59ee7eda43ad905aadbffc800206c266b30e6a1319c66dc401e5bd6b432ba49688eecd118297041da8074ce081059795679615679aa0079610079517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e01007e81517a75615779567956795679567961537956795479577995939521414136d08c5ed2bf3ba048afe6dcaebafeffffffffffffffffffffffffffffff00517951796151795179970079009f63007952799367007968517a75517a75517a7561527a75517a517951795296a0630079527994527a75517a6853798277527982775379012080517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e01205279947f7754537993527993013051797e527e54797e58797e527e53797e52797e57797e0079517a75517a75517a75517a75517a75517a75517a75517a75517a75517a75517a75517a75517a756100795779ac517a75517a75517a75517a75517a75517a75517a75517a75517a7561517a75517a756169587951797e58797eaa577961007982775179517958947f7551790128947f77517a75517a75618777777777777777777767557951876351795779a9876957795779ac777777777777777767006868")
