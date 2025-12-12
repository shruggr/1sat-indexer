package idx

import (
	"context"
	"fmt"
	"testing"

	"github.com/shruggr/1sat-indexer/v5/lib"
)

func BenchmarkSaveTxos_1(b *testing.B) {
	benchmarkSaveTxos(b, 1)
}

func BenchmarkSaveTxos_10(b *testing.B) {
	benchmarkSaveTxos(b, 10)
}

func BenchmarkSaveTxos_100(b *testing.B) {
	benchmarkSaveTxos(b, 100)
}

func benchmarkSaveTxos(b *testing.B, numOutputs int) {
	mockQueue := NewMockQueueStorage()
	mockPubSub := NewMockPubSub()
	store := NewQueueStore(mockQueue, mockPubSub)
	ctx := context.Background()

	// Pre-create outpoints
	txos := make([]*Txo, numOutputs)
	for i := 0; i < numOutputs; i++ {
		txid := fmt.Sprintf("%064d", i)
		outpoint, _ := lib.NewOutpointFromString(txid + "_0")
		sats := uint64(1000)
		txos[i] = &Txo{
			Outpoint: outpoint,
			Height:   800000,
			Idx:      uint64(i),
			Satoshis: &sats,
			Owners:   []string{"owner1"},
			Data: map[string]*IndexData{
				"ord": {
					Data: map[string]interface{}{"type": "text/plain"},
					Events: []*Event{
						{Id: "type", Value: "text/plain"},
					},
				},
			},
		}
	}

	idxCtx := &IndexContext{
		Ctx:     ctx,
		TxidHex: "0000000000000000000000000000000000000000000000000000000000000001",
		Score:   800000000000.0,
		Txos:    txos,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.SaveTxos(idxCtx)
	}
}

func BenchmarkLoadTxo(b *testing.B) {
	mockQueue := NewMockQueueStorage()
	mockPubSub := NewMockPubSub()
	store := NewQueueStore(mockQueue, mockPubSub)
	ctx := context.Background()

	// Pre-populate
	txid := "0000000000000000000000000000000000000000000000000000000000000001"
	outpoint, _ := lib.NewOutpointFromString(txid + "_0")
	sats := uint64(1000)

	idxCtx := &IndexContext{
		Ctx:     ctx,
		TxidHex: txid,
		Score:   800000000000.0,
		Txos: []*Txo{
			{
				Outpoint: outpoint,
				Height:   800000,
				Idx:      0,
				Satoshis: &sats,
				Owners:   []string{"owner1"},
				Data:     make(map[string]*IndexData),
			},
		},
	}
	store.SaveTxos(idxCtx)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.LoadTxo(ctx, outpoint.String(), nil, false)
	}
}

func BenchmarkLoadTxos_10(b *testing.B) {
	benchmarkLoadTxos(b, 10)
}

func BenchmarkLoadTxos_100(b *testing.B) {
	benchmarkLoadTxos(b, 100)
}

func benchmarkLoadTxos(b *testing.B, count int) {
	mockQueue := NewMockQueueStorage()
	mockPubSub := NewMockPubSub()
	store := NewQueueStore(mockQueue, mockPubSub)
	ctx := context.Background()

	// Pre-populate
	outpoints := make([]string, count)
	for i := 0; i < count; i++ {
		txid := fmt.Sprintf("%064d", i)
		outpoint, _ := lib.NewOutpointFromString(txid + "_0")
		sats := uint64(1000)

		idxCtx := &IndexContext{
			Ctx:     ctx,
			TxidHex: txid,
			Score:   800000000000.0 + float64(i),
			Txos: []*Txo{
				{
					Outpoint: outpoint,
					Height:   800000,
					Idx:      uint64(i),
					Satoshis: &sats,
					Owners:   []string{"owner1"},
					Data:     make(map[string]*IndexData),
				},
			},
		}
		store.SaveTxos(idxCtx)
		outpoints[i] = outpoint.String()
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.LoadTxos(ctx, outpoints, nil, false)
	}
}

func BenchmarkSaveSpends(b *testing.B) {
	mockQueue := NewMockQueueStorage()
	mockPubSub := NewMockPubSub()
	store := NewQueueStore(mockQueue, mockPubSub)
	ctx := context.Background()

	txid := "0000000000000000000000000000000000000000000000000000000000000001"
	outpoint, _ := lib.NewOutpointFromString(txid + "_0")
	sats := uint64(1000)

	// Pre-populate events for the spent output
	mockQueue.HSet(ctx, EventsKey, outpoint.String(), `["txid:`+txid+`","own:owner1"]`)

	idxCtx := &IndexContext{
		Ctx:     ctx,
		TxidHex: "0000000000000000000000000000000000000000000000000000000000000002",
		Score:   800001000000.0,
		Spends: []*Txo{
			{
				Outpoint: outpoint,
				Satoshis: &sats,
				Owners:   []string{"owner1"},
			},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.SaveSpends(idxCtx)
	}
}

func BenchmarkRollback(b *testing.B) {
	mockQueue := NewMockQueueStorage()
	mockPubSub := NewMockPubSub()
	store := NewQueueStore(mockQueue, mockPubSub)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		// Setup: create a transaction to rollback
		txid := fmt.Sprintf("%064d", i)
		outpoint, _ := lib.NewOutpointFromString(txid + "_0")
		sats := uint64(1000)

		idxCtx := &IndexContext{
			Ctx:     ctx,
			TxidHex: txid,
			Score:   800000000000.0 + float64(i),
			Txos: []*Txo{
				{
					Outpoint: outpoint,
					Height:   800000,
					Idx:      uint64(i),
					Satoshis: &sats,
					Owners:   []string{"owner1"},
					Data:     make(map[string]*IndexData),
				},
			},
		}
		store.SaveTxos(idxCtx)
		b.StartTimer()

		store.Rollback(ctx, txid)
	}
}
