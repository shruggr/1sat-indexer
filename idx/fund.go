package idx

import (
	"fmt"
	"log"
	"time"

	ec "github.com/bitcoin-sv/go-sdk/primitives/ec"
	"github.com/bitcoin-sv/go-sdk/script"
	"github.com/bitcoin-sv/go-sdk/transaction"
	feemodel "github.com/bitcoin-sv/go-sdk/transaction/fee_model"
	"github.com/bitcoin-sv/go-sdk/transaction/template/p2pkh"
	"github.com/shruggr/1sat-indexer/v5/evt"
	"github.com/shruggr/1sat-indexer/v5/jb"
	"github.com/shruggr/1sat-indexer/v5/lib"
)

const SATS_PER_KB = uint64(10)

func (idxCtx *IndexContext) FundAndSignTx(priv *ec.PrivateKey) error {
	var satsIn, satsOut uint64
	tx := idxCtx.Tx
	for _, spend := range idxCtx.Spends {
		if spend.Satoshis == nil {
			return fmt.Errorf("must load inputs")
		}
		satsIn += *spend.Satoshis
	}
	for _, out := range idxCtx.Tx.Outputs {
		satsOut += out.Satoshis
	}

	feeModel := &feemodel.SatoshisPerKilobyte{Satoshis: SATS_PER_KB}
	if fee, err := feeModel.ComputeFee(tx); err != nil {
		log.Println(err)
		return err
	} else if satsIn >= satsOut+uint64(fee+1) {
		return nil
	} else if address, err := script.NewAddressFromPublicKey(priv.PubKey(), true); err != nil {
		log.Println(err)
		return err
	} else if lockingScript, err := p2pkh.Lock(address); err != nil {
		log.Println(err)
		return err
	} else if unlock, err := p2pkh.Unlock(priv, nil); err != nil {
		log.Println(err)
		return err
	} else {
		satsNeeded := satsOut + uint64(fee+1) - satsIn
		satsIn = 0
		satsOut = satsNeeded
		fundTx := transaction.NewTransaction()
		fundTx.AddOutput(&transaction.TransactionOutput{
			LockingScript: lockingScript,
			Satoshis:      satsNeeded,
		})
		fundTx.AddOutput(&transaction.TransactionOutput{
			LockingScript: lockingScript,
			Change:        true,
		})
		if outpoints, err := idxCtx.Store.SearchMembers(idxCtx.Ctx, &SearchCfg{
			Keys:          []string{evt.EventKey("p2pkh", &evt.Event{Id: "own", Value: address.AddressString})},
			Limit:         25,
			FilterSpent:   true,
			OutpointsOnly: true,
		}); err != nil {
			log.Println(err)
			return err
		} else {
			for _, op := range outpoints {
				if outpoint, err := lib.NewOutpointFromString(op); err != nil {
					log.Println(err)
					log.Panic(err)
				} else if locked, err := jb.Cache.SetNX(idxCtx.Ctx, LockKey(op), time.Now().Unix(), time.Minute).Result(); err != nil {
					log.Println(err)
					log.Panic(err)
				} else if !locked {
					continue
				} else if txo, err := idxCtx.Store.LoadTxo(idxCtx.Ctx, op, nil, false, false); err != nil {
					log.Println(err)
					return err
				} else {
					fundTx.AddInputsFromUTXOs(&transaction.UTXO{
						TxID:                    outpoint.TxidHash(),
						Vout:                    outpoint.Vout(),
						LockingScript:           lockingScript,
						Satoshis:                *txo.Satoshis,
						UnlockingScriptTemplate: unlock,
					})
					satsIn += *txo.Satoshis
					if fundFee, err := feeModel.ComputeFee(tx); err != nil {
						log.Println(err)
						return err
					} else if satsIn >= satsOut+uint64(fundFee) {
						break
					}
				}
			}
			if err := fundTx.Fee(feeModel, transaction.ChangeDistributionEqual); err != nil {
				log.Println(err)
				return err
			} else if err := fundTx.Sign(); err != nil {
				log.Println(err)
				return err
			}
		}
		tx.AddInputFromTx(fundTx, 0, unlock)
		if err := tx.Sign(); err != nil {
			log.Println(err)
			return err
		}
	}
	return nil
}
