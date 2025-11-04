package broadcast

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/bsv-blockchain/go-sdk/transaction/broadcaster"
	"github.com/redis/go-redis/v9"
)

type registration struct {
	txid    string
	channel chan *broadcaster.ArcResponse
}

// StatusListener manages a shared Redis subscription for broadcast status updates
type StatusListener struct {
	waiters      map[string]chan *broadcaster.ArcResponse
	addWaiter    chan registration
	removeWaiter chan string
	pubsub       *redis.PubSub
	redisClient  *redis.Client
}

const arcChannel = "arc"

// Global listener instance
var Listener *StatusListener

// InitListener creates and starts the global status listener
func InitListener(redisClient *redis.Client) *StatusListener {
	Listener = &StatusListener{
		waiters:      make(map[string]chan *broadcaster.ArcResponse),
		addWaiter:    make(chan registration),
		removeWaiter: make(chan string),
		redisClient:  redisClient,
	}
	return Listener
}

// Start begins listening for status updates on Redis
func (sl *StatusListener) Start(ctx context.Context) {
	sl.pubsub = sl.redisClient.Subscribe(ctx, arcChannel)
	ch := sl.pubsub.Channel()

	go func() {
		log.Println("[ARC] Broadcast status listener started")
		for {
			select {
			case reg := <-sl.addWaiter:
				sl.waiters[reg.txid] = reg.channel

			case txid := <-sl.removeWaiter:
				if ch, ok := sl.waiters[txid]; ok {
					close(ch)
					delete(sl.waiters, txid)
				}

			case msg := <-ch:
				if msg == nil {
					log.Println("[ARC] Redis channel closed, exiting status listener")
					return
				}

				// Parse the message
				var arcResp broadcaster.ArcResponse
				if err := json.Unmarshal([]byte(msg.Payload), &arcResp); err != nil {
					log.Printf("[ARC] Error parsing ARC status update: %v", err)
					continue
				}

				// Extract txid from the message itself
				if arcResp.Txid == "" {
					log.Printf("[ARC] ARC status update missing txid: %v", arcResp)
					continue
				}

				// Send to waiter if registered
				if waiter, ok := sl.waiters[arcResp.Txid]; ok {
					select {
					case waiter <- &arcResp:
						log.Printf("[ARC] %s Delivered status update: %v", arcResp.Txid, arcResp.TxStatus)
					default:
						log.Printf("[ARC] %s Waiter channel full, skipping", arcResp.Txid)
					}
				}

			case <-ctx.Done():
				log.Println("[ARC] Context cancelled, shutting down status listener")
				sl.pubsub.Close()
				return
			}
		}
	}()
}

// RegisterTxid registers a waiter for status updates for a specific txid
// Returns a channel that will receive the ArcResponse when a status update arrives
func (sl *StatusListener) RegisterTxid(txid string, timeout time.Duration) chan *broadcaster.ArcResponse {
	ch := make(chan *broadcaster.ArcResponse, 1) // Buffered to avoid blocking
	sl.addWaiter <- registration{
		txid:    txid,
		channel: ch,
	}

	// Set up automatic cleanup after timeout
	go func() {
		time.Sleep(timeout)
		sl.UnregisterTxid(txid)
	}()

	return ch
}

// UnregisterTxid removes a waiter for a specific txid
func (sl *StatusListener) UnregisterTxid(txid string) {
	sl.removeWaiter <- txid
}
