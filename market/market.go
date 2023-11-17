package main

import (
	"bytes"
	"context"
	"encoding/hex"

	"github.com/libsv/go-bt/v2"
	"github.com/libsv/go-bt/v2/bscript"
	"github.com/shruggr/1sat-indexer/indexer"
	"github.com/shruggr/1sat-indexer/lib"
)

type Listing struct {
	PKHash []byte `json:"-"`
	Price  uint64 `json:"price"`
	PayOut []byte `json:"payout"`
}

func main() {
	err := indexer.Exec(
		true,
		true,
		handleTx,
		handleBlock,
	)
	if err != nil {
		panic(err)
	}
}

func handleTx(ctx *lib.IndexContext) error {
	for vout, txo := range ctx.Txos {
		listing := ParseScript(*txo.Tx.Outputs[vout].LockingScript)
		if listing != nil {
			save(txo, listing)
		}
	}
	return nil
}

func handleBlock(height uint32) error {
	return nil
}

var SCryptPrefix, _ = hex.DecodeString("2097dfd76851bf465e8f715593b217714858bbe9570ff3bd5e33840a34e20ff0262102ba79df5f8ae7604a9830f03c7933028186aede0675a16f025dc4f8be8eec0382201008ce7480da41702918d1ec8e6849ba32b4d65b1e40dc669c31a1e6306b266c0000")
var OrdLockSuffix, _ = hex.DecodeString("615179547a75537a537a537a0079537a75527a527a7575615579008763567901c161517957795779210ac407f0e4bd44bfc207355a778b046225a7068fc59ee7eda43ad905aadbffc800206c266b30e6a1319c66dc401e5bd6b432ba49688eecd118297041da8074ce081059795679615679aa0079610079517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e01007e81517a75615779567956795679567961537956795479577995939521414136d08c5ed2bf3ba048afe6dcaebafeffffffffffffffffffffffffffffff00517951796151795179970079009f63007952799367007968517a75517a75517a7561527a75517a517951795296a0630079527994527a75517a6853798277527982775379012080517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f517f7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e7c7e01205279947f7754537993527993013051797e527e54797e58797e527e53797e52797e57797e0079517a75517a75517a75517a75517a75517a75517a75517a75517a75517a75517a75517a75517a756100795779ac517a75517a75517a75517a75517a75517a75517a75517a75517a7561517a75517a756169587951797e58797eaa577961007982775179517958947f7551790128947f77517a75517a75618777777777777777777767557951876351795779a9876957795779ac777777777777777767006868")

func ParseScript(script []byte) *Listing {
	sCryptPrefixIndex := bytes.Index(script, SCryptPrefix)
	if sCryptPrefixIndex > -1 {
		ordLockSuffixIndex := bytes.Index(script, OrdLockSuffix)
		if ordLockSuffixIndex > -1 {
			ordLock := script[sCryptPrefixIndex+len(SCryptPrefix) : ordLockSuffixIndex]
			if ordLockParts, err := bscript.DecodeParts(ordLock); err == nil && len(ordLockParts) > 0 {
				pkhash := ordLockParts[0]
				payOutput := &bt.Output{}
				_, err = payOutput.ReadFrom(bytes.NewReader(ordLockParts[1]))
				if err == nil {
					return &Listing{
						PKHash: pkhash,
						Price:  payOutput.Satoshis,
						PayOut: payOutput.Bytes(),
					}
				}
			}
		}
	}
	return nil
}

func save(t *lib.Txo, listing *Listing) {
	_, err := indexer.Db.Exec(context.Background(), `
		INSERT INTO listings(txid, vout, height, idx, price, payout, origin, num, spend, pkhash, data, bsv20)
		SELECT $1, $2, $3, $4, $5, $6, t.origin, n.num, t.spend, t.pkhash, o.data,
			CASE WHEN t.data->'bsv20' IS NULL THEN false ELSE true END
		FROM txos t
		JOIN txos o ON o.outpoint = t.origin
		JOIN origins n ON n.origin = t.origin
		WHERE t.txid=$1 AND t.vout=$2
		ON CONFLICT(txid, vout) DO UPDATE SET 
			height=EXCLUDED.height,
			idx=EXCLUDED.idx,
			origin=EXCLUDED.origin,
			num=EXCLUDED.num`,
		t.Outpoint.Txid(),
		t.Outpoint.Vout(),
		t.Height,
		t.Idx,
		listing.Price,
		listing.PayOut,
	)

	if err != nil {
		panic(err)
	}
}
