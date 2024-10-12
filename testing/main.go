package main

import (
	"context"
	"encoding/json"
	"log"
	"os"

	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/bopen"
	"github.com/shruggr/1sat-indexer/ingest"
	"github.com/shruggr/1sat-indexer/lib"
)

var hexId = "ddea8c4e5b88e5c07f7531188ae345e4a6d0fe6dadedb5c27854caae411e3852"

func main() {
	// var err error
	err := godotenv.Load("../.env")
	if err != nil {
		log.Panic(err)
	}
	log.Println("POSTGRES_FULL:", os.Getenv("POSTGRES_FULL"))

	// db, err := pgxpool.New(context.Background(), os.Getenv("POSTGRES_FULL"))
	// if err != nil {
	// 	log.Panic(err)
	// }
	// defer lib.Db.Close()

	rdb := redis.NewClient(&redis.Options{
		Addr:     os.Getenv("REDISDB"),
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	cache := redis.NewClient(&redis.Options{
		Addr:     os.Getenv("REDISCACHE"),
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	err = lib.Initialize(nil, rdb, cache)
	if err != nil {
		log.Panic(err)
	}

	ctx := context.Background()

	indexers := []lib.Indexer{
		&bopen.BOpenIndexer{},
		&bopen.InscriptionIndexer{},
		&bopen.MapIndexer{},
		&bopen.BIndexer{},
		&bopen.SigmaIndexer{},
		&bopen.OriginIndexer{},
		// &bopen.Bsv21Indexer{},
		// &bopen.Bsv20Indexer{},
	}

	if tx, err := lib.LoadTx(ctx, hexId); err != nil {
		log.Panic(err)
	} else if idxCtx, err := ingest.IngestTx(ctx, tx, indexers); err != nil {
		log.Panic(err)
	} else if out, err := json.MarshalIndent(idxCtx, "", "  "); err != nil {
		log.Panic(err)
	} else {
		log.Println(string(out))
	}
	// scores, err := rdb.ZMScore(context.Background(), "txqueue", "1", "2").Result()

	// tx, err := lib.JB.GetTransaction(context.Background(), hexId)
	// if err != nil {
	// 	log.Panic(err)
	// }
	// // rawtx, _ := hex.DecodeString("0100000001b09d57eb0ba3a02c4e937b63d22f3202d2ac0049731af20b1e73af8a52fe3b71020000006a47304402205baf52a7fcb79dd48476a7f45943d644e96c3442be5c664e3461e91aa30f58ca02204c5df15c98f6da4918f95fe654f88f10b05007e2a8af0fab0ebc10a9d466bd54412102ab4792a94ecae6c5e4274c18a4b0274af31ddfa3c8159e9bdcf8d79b912d7383ffffffff0301000000000000008176a91417a1d887cdd2f17c693918052f7545cb87f0a83488ac0063036f726451126170706c69636174696f6e2f6273762d3230004b7b2270223a226273762d3230222c226f70223a226465706c6f792b6d696e74222c2269636f6e223a225f31222c2273796d223a224a414d222c22616d74223a22313131313131313131227d680000000000000000fd8503006a2231394878696756345179427633744870515663554551797131707a5a56646f4175744d48033c7376672077696474683d2233343222206865696768743d22333432222076696577426f783d223020302033343220333432222066696c6c3d226e6f6e652220786d6c6e733d22687474703a2f2f7777772e77332e6f72672f323030302f737667223e0a3c636972636c652063783d22313731222063793d223137312220723d22313731222066696c6c3d22626c61636b222f3e0a3c7265637420783d223138332220793d22313538222077696474683d22373522206865696768743d223236222072783d223132222066696c6c3d2223393535304635222f3e0a3c7265637420783d223137312220793d223131312e35222077696474683d22373522206865696768743d223236222072783d22313222207472616e73666f726d3d22726f74617465282d333020313731203131312e3529222066696c6c3d2223353039424635222f3e0a3c7265637420783d223138342220793d22323038222077696474683d22373522206865696768743d223236222072783d22313222207472616e73666f726d3d22726f74617465283330203138342032303829222066696c6c3d2223414632383936222f3e0a3c7061746820643d224d313438203234322e313643313438203234342e303636203134352e353838203234342e383932203134342e3432203234332e3338364c3131322e363337203230322e343238433131322e323538203230312e3934203131312e363735203230312e363534203131312e303537203230312e3635344838304337382e38393534203230312e363534203738203230302e373539203738203139392e363534563134342e303134433738203134322e39312037382e38393534203134322e303134203830203134322e303134483131302e353431433131312e313539203134322e303134203131312e373433203134312e373239203131322e313231203134312e32344c3134342e34322039392e36313431433134352e3538382039382e31303836203134382039382e3933343620313438203130302e3834563234322e31365a222066696c6c3d222339353530463522207374726f6b653d222339353530463522207374726f6b652d77696474683d22313822207374726f6b652d6c696e656a6f696e3d22726f756e64222f3e0a3c2f7376673e0a0d696d6167652f7376672b786d6c0662696e617279ebe43a00000000001976a91423a515c2179a8929a11b542ad7bd79f37e72142c88ac00000000")

	// txnCtx, err := lib.ParseTxn(tx.Transaction, "", tx.BlockHeight, tx.BlockIndex)
	// if err != nil {
	// 	log.Panic(err)
	// }
	// ordinals.ParseInscriptions(txnCtx)

	// // opns.ParseOpNS(txnCtx)
	// // ordinals.ParseBsv20(txnCtx)
	// // ordlock.ParseOrdinalLocks(txnCtx)
	// // lock.ParseLocks(txnCtx)
	// // sigil.ParseSigil(txnCtx)

	// out, err := json.MarshalIndent(txnCtx, "", "  ")
	// if err != nil {
	// 	log.Panic(err)
	// }

	// // log.Println(current.String())

	// // op, err := lib.NewOutpointFromString("2ec2781d815226e925747246b4c10730269da0e431f9edafcd6c12d8726434c6_0")
	// // if err != nil {
	// // 	log.Panic(err)
	// // }
	// // current, err := ordinals.GetLatestOutpoint(context.Background(), op)
	// // if err != nil {
	// // 	log.Panic(err)
	// // }

	// fmt.Println(scores)
}
