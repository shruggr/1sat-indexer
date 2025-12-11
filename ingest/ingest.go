package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/b-open-io/overlay/beef"
	"github.com/bsv-blockchain/go-sdk/chainhash"
	"github.com/bsv-blockchain/go-sdk/transaction"
	"github.com/bsv-blockchain/go-sdk/transaction/broadcaster"
	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/v5/config"
	"github.com/shruggr/1sat-indexer/v5/idx"
)

var ingest *idx.IngestCtx
var immutableScore float64
var arc *broadcaster.Arc

// OldMempoolTimeout is the duration after which mempool transactions without proof should be rolled back
const OldMempoolTimeout = 3 * time.Hour

func Start(ctx context.Context, ingestCtx *idx.IngestCtx, bcast *broadcaster.Arc, redisClient *redis.Client, rollback bool) {
	ingest = ingestCtx
	arc = bcast

	// Start queue processor
	go processQueue(ctx, ingestCtx)

	// Start Arc callback listener
	go startArcCallbackListener(ctx, redisClient)

	// Subscribe to chain tip updates from chaintracks
	tipChan := config.Chaintracks.SubscribeTip(ctx)
	for chaintip := range tipChan {
		log.Println("Chaintip", chaintip.Height, chaintip.Hash)
		immutableScore = idx.HeightScore(chaintip.Height-10, 0)
		AuditTransactions(ctx, rollback)
	}
}

func startArcCallbackListener(ctx context.Context, redisClient *redis.Client) {
	pubsub := redisClient.Subscribe(ctx, "arc")
	defer pubsub.Close()

	ch := pubsub.Channel()
	log.Println("Arc callback listener started for ingestion")

	for {
		select {
		case msg := <-ch:
			if msg == nil {
				log.Println("Arc callback channel closed, exiting ingestion listener")
				return
			}

			var arcResp broadcaster.ArcResponse
			if err := json.Unmarshal([]byte(msg.Payload), &arcResp); err != nil {
				log.Printf("Error parsing Arc callback in ingestion: %v", err)
				continue
			}

			if arcResp.Txid == "" || arcResp.TxStatus == nil {
				continue
			}

			log.Printf("Arc callback received in ingestion: txid=%s, status=%s", arcResp.Txid, *arcResp.TxStatus)

			// Scenario 1: Arc Success - Transaction mined with MerklePath
			if arcResp.MerklePath != "" {
				go func(txidStr, merklePath string) {
					txidHash, _ := chainhash.NewHashFromHex(txidStr)
					tx, err := config.BeefStorage.LoadTx(ctx, txidHash)
					if err != nil {
						log.Printf("Error loading tx %s: %v", txidStr, err)
						return
					}

					if tx.MerklePath, err = transaction.NewMerklePathFromHex(merklePath); err != nil {
						log.Printf("Error parsing MerklePath for %s: %v", txidStr, err)
						return
					}

					if err := reprocessTransaction(ctx, tx.TxID(), tx); err != nil {
						log.Printf("Error reprocessing tx %s: %v", txidStr, err)
					}
				}(arcResp.Txid, arcResp.MerklePath)
				continue
			}

			// Scenario 2: Arc Error - Transaction rejected
			status := broadcaster.ArcStatus(*arcResp.TxStatus)
			if status == broadcaster.REJECTED ||
				status == broadcaster.DOUBLE_SPEND_ATTEMPTED ||
				status == broadcaster.SEEN_IN_ORPHAN_MEMPOOL {
				go func(txidStr string) {
					if err := ingest.Store.Rollback(ctx, txidStr); err != nil {
						log.Printf("Error rolling back tx %s: %v", txidStr, err)
						return
					}
					if err := ingest.Store.Log(ctx, idx.RollbackTxLog, txidStr, 0); err != nil {
						log.Printf("Error logging rollback for %s: %v", txidStr, err)
					}
				}(arcResp.Txid)
			}

		case <-ctx.Done():
			log.Println("Context cancelled, shutting down Arc ingestion listener")
			return
		}
	}
}

func reprocessTransaction(ctx context.Context, txid *chainhash.Hash, tx *transaction.Transaction) error {
	var newScore float64
	if root, err := tx.MerklePath.ComputeRoot(txid); err != nil {
		log.Println("ComputeRoot error", txid, err)
		return err
	} else if valid, err := config.Chaintracks.IsValidRootForHeight(ctx, root, tx.MerklePath.BlockHeight); err != nil {
		log.Println("IsValidRootForHeight error", txid, err)
		return err
	} else if !valid {
		log.Println("Invalid proof for", txid)
		return nil
	}

	for _, path := range tx.MerklePath.Path[0] {
		if txid.IsEqual(path.Hash) {
			newScore = idx.HeightScore(tx.MerklePath.BlockHeight, path.Offset)
			break
		}
	}

	if newScore == 0 {
		log.Println("Transaction not in proof", txid)
		return nil
	}

	log.Println("Reingest", txid, newScore)
	if _, err := ingest.IngestTx(ctx, tx); err != nil {
		log.Println("IngestTx error", txid, err)
		return err
	}

	if newScore < immutableScore {
		log.Println("Archive Immutable", txid, newScore)
		if err := ingest.Store.Log(ctx, idx.ImmutableTxLog, txid.String(), newScore); err != nil {
			log.Println("Log error", txid, err)
			return err
		} else if err := ingest.Store.Delog(ctx, idx.PendingTxLog, txid.String()); err != nil {
			log.Println("Delog error", txid, err)
			return err
		}
	}
	return nil
}

func AuditTransactions(ctx context.Context, rollback bool) {
	start := time.Now()
	log.Println("[AUDIT] Starting transaction audit")

	limiter := make(chan struct{}, 20)
	var wg sync.WaitGroup

	// Scenario 1: Negative scores - Broadcast attempts that never got Arc confirmation (older than 2 minutes)
	// Just check if Arc has it. If not, delete. If yes, it will be picked up by Arc callbacks.
	auditNegativeScores(ctx, &wg, limiter)

	// Scenario 2: Mined transactions (0 to immutableScore) - Verify MerklePath and check for immutability
	auditMinedTransactions(ctx, rollback, &wg, limiter)

	// Scenario 3: Old mempool transactions (>OldMempoolTimeout) - Check if they got mined
	auditMempool(ctx, rollback, &wg, limiter)

	wg.Wait()
	log.Printf("[AUDIT] Completed transaction audit (%.2fs)", time.Since(start).Seconds())
}

func auditNegativeScores(ctx context.Context, wg *sync.WaitGroup, limiter chan struct{}) {
	from := float64(-1 * time.Now().Add(-2*time.Minute).UnixNano())
	to := 0.0
	cfg := &idx.SearchCfg{
		Keys:  []string{idx.PendingTxLog},
		From:  &from,
		To:    &to,
		Limit: 100000,
	}

	items, err := ingest.Store.Search(ctx, cfg)
	if err != nil {
		log.Println("[AUDIT] Search negative scores error:", err)
		return
	}

	count := len(items)
	if count > 0 {
		log.Printf("[AUDIT] Checking %d unconfirmed broadcast txs (>2min old)", count)
	}
	for _, item := range items {
		select {
		case <-ctx.Done():
			log.Println("Context cancelled during negative scores audit")
			return
		case limiter <- struct{}{}:
		}

		wg.Add(1)
		go func(txid string) {
			defer func() {
				<-limiter
				wg.Done()
			}()

			// Check if transaction exists in BeefStorage
			txidHash, _ := chainhash.NewHashFromHex(txid)
			_, err := config.BeefStorage.LoadTx(ctx, txidHash)
			if errors.Is(err, beef.ErrNotFound) {
				log.Println("Archive Missing", txid)
				if err := ingest.Store.Delog(ctx, idx.PendingTxLog, txid); err != nil {
					log.Printf("Delog error for %s: %v", txid, err)
				}
				return
			} else if err != nil {
				log.Printf("LoadTx error for %s: %v", txid, err)
				return
			}

			// Check if Arc has the transaction
			status, err := arc.Status(txid)
			if err != nil {
				log.Printf("Arc status check error for %s: %v", txid, err)
				return
			}

			if status.Status == 404 {
				// Arc doesn't have it - remove from PendingTxLog
				// Could consider rebroadcasting in the future
				log.Println("Removing unconfirmed tx:", txid)
				if err := ingest.Store.Delog(ctx, idx.PendingTxLog, txid); err != nil {
					log.Printf("Error removing %s from PendingTxLog: %v", txid, err)
				}
			} else if status.Status == 200 {
				// Arc has it - publish to Redis, Arc callback listener will handle ingestion
				if jsonData, err := json.Marshal(status); err != nil {
					log.Printf("Error marshaling Arc status for %s: %v", txid, err)
				} else if err := config.PubSub.Publish(ctx, "arc", string(jsonData)); err != nil {
					log.Printf("Error publishing Arc status for %s: %v", txid, err)
				}
			}
		}(item.Member)
	}
}

func auditMinedTransactions(ctx context.Context, rollback bool, wg *sync.WaitGroup, limiter chan struct{}) {
	from := 0.0
	cfg := &idx.SearchCfg{
		Keys:  []string{idx.PendingTxLog},
		From:  &from,
		To:    &immutableScore,
		Limit: 100000,
	}

	items, err := ingest.Store.Search(ctx, cfg)
	if err != nil {
		log.Println("[AUDIT] Search mined txs error:", err)
		return
	}

	count := len(items)
	if count > 0 {
		log.Printf("[AUDIT] Verifying %d mined txs for immutability", count)
	}
	for _, item := range items {
		select {
		case <-ctx.Done():
			log.Println("Context cancelled during mined transactions audit")
			return
		case limiter <- struct{}{}:
		}

		wg.Add(1)
		go func(txid string, score float64) {
			defer func() {
				<-limiter
				wg.Done()
			}()

			// Load transaction with MerklePath
			txidHash, _ := chainhash.NewHashFromHex(txid)
			tx, err := config.BeefStorage.LoadTx(ctx, txidHash)
			if errors.Is(err, beef.ErrNotFound) {
				// Transaction was supposedly mined >10 blocks ago but doesn't exist
				// This is a reorg victim - rollback
				if rollback {
					log.Printf("Rolling back missing mined tx (>10 blocks): %s", txid)
					if err := ingest.Store.Rollback(ctx, txid); err != nil {
						log.Printf("Rollback error for %s: %v", txid, err)
						return
					}
					if err := ingest.Store.Log(ctx, idx.RollbackTxLog, txid, score); err != nil {
						log.Printf("Log to RollbackTxLog error for %s: %v", txid, err)
					}
				} else {
					log.Printf("Missing mined tx (>10 blocks), rollback disabled: %s", txid)
				}
				return
			} else if err != nil {
				// Other errors (network, timeout, etc.) - skip and retry later
				log.Printf("LoadTx error for %s: %v", txid, err)
				return
			}

			valid := false

			// Validate MerklePath from JungleBus (if present)
			if tx.MerklePath != nil {
				root, err := tx.MerklePath.ComputeRoot(txidHash)
				if err != nil {
					log.Printf("ComputeRoot error for %s: %v", txid, err)
					return
				}
				valid, err = config.Chaintracks.IsValidRootForHeight(ctx, root, tx.MerklePath.BlockHeight)
				if err != nil {
					log.Printf("IsValidRootForHeight error for %s: %v", txid, err)
					return
				}
			}

			// If not present or not valid, try Arc
			if !valid {
				status, err := arc.Status(txid)
				if err != nil {
					log.Printf("Arc status error for %s: %v", txid, err)
					return
				}
				if status.MerklePath != "" {
					tx.MerklePath, err = transaction.NewMerklePathFromHex(status.MerklePath)
					if err != nil {
						log.Printf("Error parsing Arc MerklePath for %s: %v", txid, err)
						return
					}
					root, err := tx.MerklePath.ComputeRoot(txidHash)
					if err != nil {
						log.Printf("ComputeRoot error for Arc MerklePath %s: %v", txid, err)
						return
					}
					valid, err = config.Chaintracks.IsValidRootForHeight(ctx, root, tx.MerklePath.BlockHeight)
					if err != nil {
						log.Printf("IsValidRootForHeight error for Arc MerklePath %s: %v", txid, err)
						return
					}
				}
			}

			// If still not present or not valid after checking both providers
			// This transaction was mined >10 blocks ago but no longer has a valid proof
			// This indicates it was reorged out and not re-mined - should be rolled back
			if !valid {
				if rollback {
					log.Printf("Rolling back reorg victim (>10 blocks): %s", txid)
					if err := ingest.Store.Rollback(ctx, txid); err != nil {
						log.Printf("Rollback error for %s: %v", txid, err)
						return
					}
					if err := ingest.Store.Log(ctx, idx.RollbackTxLog, txid, score); err != nil {
						log.Printf("Log to RollbackTxLog error for %s: %v", txid, err)
					}
				} else {
					log.Printf("Reorg victim (>10 blocks) without proof, rollback disabled: %s", txid)
				}
				return
			}

			// Re-ingest to recalculate score from MerklePath
			log.Printf("Re-ingesting mined tx: %s", txid)
			if _, err := ingest.IngestTx(ctx, tx); err != nil {
				log.Printf("IngestTx error for %s: %v", txid, err)
				return
			}

			// Check if transaction is now immutable (>10 blocks deep)
			newScore := idx.HeightScore(tx.MerklePath.BlockHeight, 0)
			if newScore < immutableScore {
				log.Printf("Archiving immutable tx: %s", txid)
				if err := ingest.Store.Log(ctx, idx.ImmutableTxLog, txid, newScore); err != nil {
					log.Printf("Log to ImmutableTxLog error for %s: %v", txid, err)
					return
				}
				if err := ingest.Store.Delog(ctx, idx.PendingTxLog, txid); err != nil {
					log.Printf("Delog from PendingTxLog error for %s: %v", txid, err)
				}
			}
		}(item.Member, item.Score)
	}
}

func auditMempool(ctx context.Context, rollback bool, wg *sync.WaitGroup, limiter chan struct{}) {
	// Get mempool transactions older than 2 minutes
	to := float64(time.Now().Add(-2 * time.Minute).UnixNano())
	cfg := &idx.SearchCfg{
		Keys:  []string{idx.PendingTxLog},
		From:  &idx.MempoolScore,
		To:    &to,
		Limit: 100000,
	}

	items, err := ingest.Store.Search(ctx, cfg)
	if err != nil {
		log.Println("[AUDIT] Search mempool txs error:", err)
		return
	}

	count := len(items)
	if count > 0 {
		log.Printf("[AUDIT] Checking %d mempool txs (>2min old)", count)
	}
	for _, item := range items {
		select {
		case <-ctx.Done():
			log.Println("Context cancelled during old mempool audit")
			return
		case limiter <- struct{}{}:
		}

		wg.Add(1)
		go func(txid string, score float64) {
			defer func() {
				<-limiter
				wg.Done()
			}()

			// Determine age of mempool transaction (score is UnixNano timestamp)
			txTime := time.Unix(0, int64(score))
			age := time.Since(txTime)
			isOld := age > OldMempoolTimeout

			// Load transaction with proof
			txidHash, _ := chainhash.NewHashFromHex(txid)
			tx, err := config.BeefStorage.LoadTx(ctx, txidHash)
			if errors.Is(err, beef.ErrNotFound) {
				// Transaction doesn't exist in BeefStorage
				// All transactions here are >2min old (filtered by query)
				if isOld {
					// >3hr old: Rollback if flag enabled
					if rollback {
						log.Printf("Rolling back missing mempool tx (age: %v): %s", age.Round(time.Second), txid)
						if err := ingest.Store.Rollback(ctx, txid); err != nil {
							log.Printf("Rollback error for %s: %v", txid, err)
							return
						}
						if err := ingest.Store.Log(ctx, idx.RollbackTxLog, txid, score); err != nil {
							log.Printf("Log to RollbackTxLog error for %s: %v", txid, err)
						}
					} else {
						log.Printf("Missing mempool tx (age: %v), rollback disabled: %s", age.Round(time.Second), txid)
					}
				} else {
					// 2min-3hr old: Delog from queue (transaction must exist if >2min)
					log.Printf("Removing missing mempool tx (age: %v) from queue: %s", age.Round(time.Second), txid)
					if err := ingest.Store.Delog(ctx, idx.PendingTxLog, txid); err != nil {
						log.Printf("Delog error for %s: %v", txid, err)
					}
				}
				return
			} else if err != nil {
				log.Printf("LoadTx error for %s: %v", txid, err)
				return
			}

			valid := false

			// Validate MerklePath from JungleBus (if present)
			if tx.MerklePath != nil {
				root, err := tx.MerklePath.ComputeRoot(txidHash)
				if err != nil {
					log.Printf("ComputeRoot error for %s: %v", txid, err)
					return
				}
				valid, err = config.Chaintracks.IsValidRootForHeight(ctx, root, tx.MerklePath.BlockHeight)
				if err != nil {
					log.Printf("IsValidRootForHeight error for %s: %v", txid, err)
					return
				}
			}

			// If not present or not valid, try Arc
			if !valid {
				status, err := arc.Status(txid)
				if err != nil {
					log.Printf("Arc status error for %s: %v", txid, err)
					return
				}
				if status.MerklePath != "" {
					tx.MerklePath, err = transaction.NewMerklePathFromHex(status.MerklePath)
					if err != nil {
						log.Printf("Error parsing Arc MerklePath for %s: %v", txid, err)
						return
					}
					root, err := tx.MerklePath.ComputeRoot(txidHash)
					if err != nil {
						log.Printf("ComputeRoot error for Arc MerklePath %s: %v", txid, err)
						return
					}
					valid, err = config.Chaintracks.IsValidRootForHeight(ctx, root, tx.MerklePath.BlockHeight)
					if err != nil {
						log.Printf("IsValidRootForHeight error for Arc MerklePath %s: %v", txid, err)
						return
					}
				}

			}
			// If still not present or not valid after checking both providers
			// This transaction has been in mempool for >OldMempoolTimeout without being mined
			// If rollback flag is enabled, remove it from the index
			if !valid {
				if rollback {
					log.Printf("Rolling back old mempool tx (>%v): %s", OldMempoolTimeout, txid)
					if err := ingest.Store.Rollback(ctx, txid); err != nil {
						log.Printf("Rollback error for %s: %v", txid, err)
						return
					}
					if err := ingest.Store.Log(ctx, idx.RollbackTxLog, txid, score); err != nil {
						log.Printf("Log to RollbackTxLog error for %s: %v", txid, err)
					}
				} else {
					log.Printf("Old mempool tx (>%v) without proof, rollback disabled: %s", OldMempoolTimeout, txid)
				}
				return
			}

			// Re-ingest with MerklePath
			log.Printf("Re-ingesting old mempool tx: %s", txid)
			if _, err := ingest.IngestTx(ctx, tx); err != nil {
				log.Printf("IngestTx error for %s: %v", txid, err)
				return
			}

			// Check if now immutable
			newScore := idx.HeightScore(tx.MerklePath.BlockHeight, 0)
			if newScore < immutableScore {
				log.Printf("Archiving immutable tx: %s", txid)
				if err := ingest.Store.Log(ctx, idx.ImmutableTxLog, txid, newScore); err != nil {
					log.Printf("Log to ImmutableTxLog error for %s: %v", txid, err)
					return
				}
				if err := ingest.Store.Delog(ctx, idx.PendingTxLog, txid); err != nil {
					log.Printf("Delog from PendingTxLog error for %s: %v", txid, err)
				}
			}
		}(item.Member, item.Score)
	}
}

func processQueue(ctx context.Context, cfg *idx.IngestCtx) {
	limiter := make(chan struct{}, cfg.Concurrency)
	errChan := make(chan error)
	done := make(chan string)
	inflight := make(map[string]struct{})
	ticker := time.NewTicker(15 * time.Second)
	ingestcount := 0
	txcount := uint32(0)
	statusTime := time.Now()
	wg := sync.WaitGroup{}
	lastScore := float64(0)

	for {
		select {
		case <-ctx.Done():
			log.Println("Context cancelled, shutting down queue processor")
			wg.Wait()
			return

		case now := <-ticker.C:
			duration := time.Since(statusTime)
			log.Printf("Ingested %d in %ds - %.02ftx/s height %d", ingestcount, int(duration.Seconds()), float64(ingestcount)/duration.Seconds(), int(lastScore/1000000000))
			ingestcount = 0
			statusTime = now

		case txid := <-done:
			delete(inflight, txid)
			ingestcount++

		case err := <-errChan:
			log.Println("Queue processing error:", err)
			if cfg.Once {
				return
			}

		default:
			to := float64(time.Now().UnixNano())
			if logs, err := cfg.Store.Search(ctx, &idx.SearchCfg{
				Keys:  []string{cfg.Key},
				Limit: cfg.PageSize,
				To:    &to,
			}); err != nil {
				log.Println("Queue search error:", err)
				time.Sleep(time.Second)
			} else {
				if len(logs) == 0 {
					if cfg.Verbose {
						log.Println("No transactions to ingest")
					}
					time.Sleep(time.Second)
				}
				for _, l := range logs {
					txid := l.Member
					lastScore = l.Score
					if _, ok := inflight[txid]; !ok {
						txcount++
						if cfg.Limit > 0 && txcount > cfg.Limit {
							wg.Wait()
							return
						}
						inflight[txid] = struct{}{}
						limiter <- struct{}{}
						wg.Add(1)
						go func(txid string, score float64) {
							defer func() {
								if r := recover(); r != nil {
									log.Println("Recovered from panic in queue processor:", r)
								}
								<-limiter
								wg.Done()
								done <- txid
							}()
							txidHash, _ := chainhash.NewHashFromHex(txid)
							tx, err := config.BeefStorage.LoadTx(ctx, txidHash)
							if errors.Is(err, beef.ErrNotFound) {
								// Transaction not found in BeefStorage - remove from queue
								log.Printf("[QUEUE] Transaction not found, removing from queue: %s", txid)
								if err := cfg.Store.Delog(ctx, cfg.Key, txid); err != nil {
									log.Printf("Delog error for %s: %v", txid, err)
								}
								return
							} else if err != nil {
								log.Printf("LoadTx error %s: %v", txid, err)
								errChan <- err
							} else if idxCtx, err := cfg.IngestTx(ctx, tx); err != nil {
								log.Printf("Ingest error %s: %v", txid, err)
								errChan <- err
							} else if cfg.OnIngest != nil {
								if err := (*cfg.OnIngest)(ctx, idxCtx); err != nil {
									errChan <- err
								}
							} else if len(cfg.Key) > 0 {
								if err = cfg.Store.Delog(ctx, cfg.Key, txid); err != nil {
									log.Println("Delog error:", err)
									return
								}
							}
						}(txid, l.Score)
					}
				}
			}
		}
	}
}
