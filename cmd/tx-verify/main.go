package main

// var store = redisstore.NewRedisTxoStore(os.Getenv("REDISTXO"))
// var ctx = context.Background()

func main() {
	// if mempool, err := store.Search(ctx, &idx.SearchCfg{
	// 	Key:   idx.TxLogTag,
	// 	From:  idx.HeightScore(50000000, 0),
	// 	Limit: 10,
	// }); err != nil {
	// 	panic(err)
	// } else {
	// 	var ct chaintracker.ChainTracker
	// 	for _, ptr := range mempool {
	// 		if tx, err := jb.LoadTx(ctx, ptr.Member, true); err != nil {
	// 			panic(err)
	// 		} else if tx.MerklePath == nil {
	// 			panic("tx.MerklePath is nil")
	// 		} else {
	// 			tx.MerklePath.Verify(tx.TxID().String())
	// 		}
	// 	}
	// }
}
