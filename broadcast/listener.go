package broadcast

import (
	"context"
	"encoding/json"
	"log"
	"sync"
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
	mu            sync.Mutex
	waiters       map[string]chan *broadcaster.ArcResponse
	addWaiter     chan registration
	removeWaiter  chan string
	pubsub        *redis.PubSub
	redisClient   *redis.Client
}

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
	sl.pubsub = sl.redisClient.Subscribe(ctx)
	ch := sl.pubsub.Channel()

	go func() {
		log.Println("Broadcast status listener started")
		for {
			select {
			case reg := <-sl.addWaiter:
				sl.mu.Lock()
				sl.waiters[reg.txid] = reg.channel
				// Subscribe to the specific channel for this txid
				channel := "stat:" + reg.txid
				if err := sl.pubsub.Subscribe(ctx, channel); err != nil {
					log.Printf("Error subscribing to %s: %v", channel, err)
				}
				sl.mu.Unlock()

			case txid := <-sl.removeWaiter:
				sl.mu.Lock()
				if ch, ok := sl.waiters[txid]; ok {
					close(ch)
					delete(sl.waiters, txid)
					// Unsubscribe from the channel
					channel := "stat:" + txid
					if err := sl.pubsub.Unsubscribe(ctx, channel); err != nil {
						log.Printf("Error unsubscribing from %s: %v", channel, err)
					}
				}
				sl.mu.Unlock()

			case msg := <-ch:
				if msg == nil {
					log.Println("Redis channel closed, exiting status listener")
					return
				}

				// Parse the message
				var arcResp broadcaster.ArcResponse
				if err := json.Unmarshal([]byte(msg.Payload), &arcResp); err != nil {
					log.Printf("Error parsing ARC status update: %v", err)
					continue
				}

				// Extract txid from channel name (format: "stat:txid")
				if len(msg.Channel) < 6 {
					log.Printf("Invalid channel format: %s", msg.Channel)
					continue
				}
				txid := msg.Channel[5:] // Remove "stat:" prefix

				// Send to waiter if registered
				sl.mu.Lock()
				if waiter, ok := sl.waiters[txid]; ok {
					select {
					case waiter <- &arcResp:
						log.Printf("Delivered status update for %s: %v", txid, arcResp.TxStatus)
					default:
						log.Printf("Waiter channel full for %s, skipping", txid)
					}
				}
				sl.mu.Unlock()

			case <-ctx.Done():
				log.Println("Context cancelled, shutting down status listener")
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
