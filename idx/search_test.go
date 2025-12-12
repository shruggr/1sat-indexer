package idx

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/b-open-io/overlay/queue"
	"github.com/shruggr/1sat-indexer/v5/lib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupSearchStore(t *testing.T) (*QueueStore, *MockQueueStorage, context.Context) {
	mockQueue := NewMockQueueStorage()
	mockPubSub := NewMockPubSub()
	store := NewQueueStore(mockQueue, mockPubSub)
	return store, mockQueue, context.Background()
}

func TestSearch_SingleKey(t *testing.T) {
	store, mockQueue, ctx := setupSearchStore(t)

	// Populate sorted set
	mockQueue.ZAdd(ctx, "test:key", queue.ScoredMember{Member: "out1_0", Score: 100})
	mockQueue.ZAdd(ctx, "test:key", queue.ScoredMember{Member: "out2_0", Score: 200})
	mockQueue.ZAdd(ctx, "test:key", queue.ScoredMember{Member: "out3_0", Score: 300})

	results, err := store.Search(ctx, &SearchCfg{
		Keys: []string{"test:key"},
	})
	require.NoError(t, err)
	require.Len(t, results, 3)

	// Should be in ascending order by default
	assert.Equal(t, "out1_0", results[0].Member)
	assert.Equal(t, "out2_0", results[1].Member)
	assert.Equal(t, "out3_0", results[2].Member)
}

func TestSearch_SingleKey_Reverse(t *testing.T) {
	store, mockQueue, ctx := setupSearchStore(t)

	mockQueue.ZAdd(ctx, "test:key", queue.ScoredMember{Member: "out1_0", Score: 100})
	mockQueue.ZAdd(ctx, "test:key", queue.ScoredMember{Member: "out2_0", Score: 200})
	mockQueue.ZAdd(ctx, "test:key", queue.ScoredMember{Member: "out3_0", Score: 300})

	results, err := store.Search(ctx, &SearchCfg{
		Keys:    []string{"test:key"},
		Reverse: true,
	})
	require.NoError(t, err)
	require.Len(t, results, 3)

	// Should be in descending order
	assert.Equal(t, "out3_0", results[0].Member)
	assert.Equal(t, "out2_0", results[1].Member)
	assert.Equal(t, "out1_0", results[2].Member)
}

func TestSearch_MultiKey_OR(t *testing.T) {
	store, mockQueue, ctx := setupSearchStore(t)

	// Populate two different keys
	mockQueue.ZAdd(ctx, "key1", queue.ScoredMember{Member: "out1_0", Score: 100})
	mockQueue.ZAdd(ctx, "key1", queue.ScoredMember{Member: "out2_0", Score: 200})
	mockQueue.ZAdd(ctx, "key2", queue.ScoredMember{Member: "out3_0", Score: 150})
	mockQueue.ZAdd(ctx, "key2", queue.ScoredMember{Member: "out4_0", Score: 250})

	results, err := store.Search(ctx, &SearchCfg{
		Keys:           []string{"key1", "key2"},
		ComparisonType: ComparisonOR,
	})
	require.NoError(t, err)
	require.Len(t, results, 4)

	// OR: union of all members, sorted by score ascending
	assert.Equal(t, "out1_0", results[0].Member) // 100
	assert.Equal(t, "out3_0", results[1].Member) // 150
	assert.Equal(t, "out2_0", results[2].Member) // 200
	assert.Equal(t, "out4_0", results[3].Member) // 250
}

func TestSearch_MultiKey_OR_WithDuplicates(t *testing.T) {
	store, mockQueue, ctx := setupSearchStore(t)

	// Same member in both keys with SAME score - should be deduped
	mockQueue.ZAdd(ctx, "key1", queue.ScoredMember{Member: "out1_0", Score: 100})
	mockQueue.ZAdd(ctx, "key2", queue.ScoredMember{Member: "out1_0", Score: 100}) // same score

	results, err := store.Search(ctx, &SearchCfg{
		Keys:           []string{"key1", "key2"},
		ComparisonType: ComparisonOR,
	})
	require.NoError(t, err)
	require.Len(t, results, 1) // deduped because same score
	assert.Equal(t, "out1_0", results[0].Member)
}

func TestSearch_MultiKey_OR_DifferentScores(t *testing.T) {
	store, mockQueue, ctx := setupSearchStore(t)

	// Same member in both keys with DIFFERENT scores - appears twice
	// This is by design: the same outpoint might have different scores
	// in different sorted sets (e.g., different block heights)
	mockQueue.ZAdd(ctx, "key1", queue.ScoredMember{Member: "out1_0", Score: 100})
	mockQueue.ZAdd(ctx, "key2", queue.ScoredMember{Member: "out1_0", Score: 150}) // different score

	results, err := store.Search(ctx, &SearchCfg{
		Keys:           []string{"key1", "key2"},
		ComparisonType: ComparisonOR,
	})
	require.NoError(t, err)
	require.Len(t, results, 2) // appears twice because different scores
}

func TestSearch_MultiKey_AND(t *testing.T) {
	store, mockQueue, ctx := setupSearchStore(t)

	// Members that exist in both keys
	mockQueue.ZAdd(ctx, "key1", queue.ScoredMember{Member: "out1_0", Score: 100})
	mockQueue.ZAdd(ctx, "key1", queue.ScoredMember{Member: "out2_0", Score: 200})
	mockQueue.ZAdd(ctx, "key1", queue.ScoredMember{Member: "out3_0", Score: 300})

	mockQueue.ZAdd(ctx, "key2", queue.ScoredMember{Member: "out2_0", Score: 200}) // intersection
	mockQueue.ZAdd(ctx, "key2", queue.ScoredMember{Member: "out3_0", Score: 300}) // intersection
	mockQueue.ZAdd(ctx, "key2", queue.ScoredMember{Member: "out4_0", Score: 400})

	results, err := store.Search(ctx, &SearchCfg{
		Keys:           []string{"key1", "key2"},
		ComparisonType: ComparisonAND,
	})
	require.NoError(t, err)
	require.Len(t, results, 2) // only intersection
	assert.Equal(t, "out2_0", results[0].Member)
	assert.Equal(t, "out3_0", results[1].Member)
}

func TestSearch_MultiKey_AND_NoIntersection(t *testing.T) {
	store, mockQueue, ctx := setupSearchStore(t)

	mockQueue.ZAdd(ctx, "key1", queue.ScoredMember{Member: "out1_0", Score: 100})
	mockQueue.ZAdd(ctx, "key2", queue.ScoredMember{Member: "out2_0", Score: 200})

	results, err := store.Search(ctx, &SearchCfg{
		Keys:           []string{"key1", "key2"},
		ComparisonType: ComparisonAND,
	})
	require.NoError(t, err)
	assert.Len(t, results, 0)
}

func TestSearch_FilterSpent(t *testing.T) {
	store, mockQueue, ctx := setupSearchStore(t)

	// Populate sorted set
	mockQueue.ZAdd(ctx, "test:key", queue.ScoredMember{Member: "out1_0", Score: 100})
	mockQueue.ZAdd(ctx, "test:key", queue.ScoredMember{Member: "out2_0", Score: 200})
	mockQueue.ZAdd(ctx, "test:key", queue.ScoredMember{Member: "out3_0", Score: 300})

	// Mark out2_0 as spent
	mockQueue.HSet(ctx, SpendsKey, "out2_0", "spendingtx")

	results, err := store.Search(ctx, &SearchCfg{
		Keys:        []string{"test:key"},
		FilterSpent: true,
	})
	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.Equal(t, "out1_0", results[0].Member)
	assert.Equal(t, "out3_0", results[1].Member)
}

func TestSearch_Pagination(t *testing.T) {
	store, mockQueue, ctx := setupSearchStore(t)

	// Populate many items
	for i := 0; i < 10; i++ {
		mockQueue.ZAdd(ctx, "test:key", queue.ScoredMember{
			Member: "out" + string(rune('0'+i)) + "_0",
			Score:  float64((i + 1) * 100),
		})
	}

	// First page
	results, err := store.Search(ctx, &SearchCfg{
		Keys:  []string{"test:key"},
		Limit: 3,
	})
	require.NoError(t, err)
	require.Len(t, results, 3)
	assert.Equal(t, float64(100), results[0].Score)
	assert.Equal(t, float64(200), results[1].Score)
	assert.Equal(t, float64(300), results[2].Score)
}

func TestSearch_ScoreRange(t *testing.T) {
	store, mockQueue, ctx := setupSearchStore(t)

	mockQueue.ZAdd(ctx, "test:key", queue.ScoredMember{Member: "out1_0", Score: 100})
	mockQueue.ZAdd(ctx, "test:key", queue.ScoredMember{Member: "out2_0", Score: 200})
	mockQueue.ZAdd(ctx, "test:key", queue.ScoredMember{Member: "out3_0", Score: 300})
	mockQueue.ZAdd(ctx, "test:key", queue.ScoredMember{Member: "out4_0", Score: 400})

	from := float64(150)
	to := float64(350)
	results, err := store.Search(ctx, &SearchCfg{
		Keys: []string{"test:key"},
		From: &from,
		To:   &to,
	})
	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.Equal(t, "out2_0", results[0].Member) // 200
	assert.Equal(t, "out3_0", results[1].Member) // 300
}

func TestSearch_Empty(t *testing.T) {
	store, _, ctx := setupSearchStore(t)

	results, err := store.Search(ctx, &SearchCfg{
		Keys: []string{"nonexistent:key"},
	})
	require.NoError(t, err)
	assert.Len(t, results, 0)
}

func TestSearch_NoKeys(t *testing.T) {
	store, _, ctx := setupSearchStore(t)

	results, err := store.Search(ctx, &SearchCfg{
		Keys: []string{},
	})
	require.NoError(t, err)
	assert.Nil(t, results)
}

func TestSearch_DefaultLimit(t *testing.T) {
	store, mockQueue, ctx := setupSearchStore(t)

	// Populate more than default limit (1000)
	for i := 0; i < 1500; i++ {
		mockQueue.ZAdd(ctx, "test:key", queue.ScoredMember{
			Member: "out" + string(rune(i)) + "_0",
			Score:  float64(i),
		})
	}

	results, err := store.Search(ctx, &SearchCfg{
		Keys: []string{"test:key"},
		// No limit specified - should use default 1000
	})
	require.NoError(t, err)
	assert.Len(t, results, 1000)
}

func TestSearchTxos(t *testing.T) {
	store, mockQueue, ctx := setupSearchStore(t)

	txid := "0000000000000000000000000000000000000000000000000000000000000001"
	outpoint, _ := lib.NewOutpointFromString(txid + "_0")

	// Pre-populate TXO
	stored := storedTxo{Height: 800000, Idx: 0, Satoshis: 1000}
	storedJSON, _ := json.Marshal(stored)
	mockQueue.HSet(ctx, TxoKey, outpoint.String(), string(storedJSON))

	// Pre-populate sorted set
	mockQueue.ZAdd(ctx, "test:key", queue.ScoredMember{Member: outpoint.String(), Score: 800000000000.0})

	txos, err := store.SearchTxos(ctx, &SearchCfg{
		Keys: []string{"test:key"},
	})
	require.NoError(t, err)
	require.Len(t, txos, 1)
	assert.Equal(t, uint64(1000), *txos[0].Satoshis)
}

func TestSearchTxos_OutpointsOnly(t *testing.T) {
	store, mockQueue, ctx := setupSearchStore(t)

	mockQueue.ZAdd(ctx, "test:key", queue.ScoredMember{Member: "out1_0", Score: 100})

	txos, err := store.SearchTxos(ctx, &SearchCfg{
		Keys:          []string{"test:key"},
		OutpointsOnly: true,
	})
	require.NoError(t, err)
	assert.Nil(t, txos)
}

func TestSearchBalance(t *testing.T) {
	store, mockQueue, ctx := setupSearchStore(t)

	// Pre-populate TXOs
	mockQueue.HSet(ctx, SatsKey, "out1_0", "100")
	mockQueue.HSet(ctx, SatsKey, "out2_0", "200")
	mockQueue.HSet(ctx, SatsKey, "out3_0", "300") // will be spent

	// Pre-populate sorted set
	mockQueue.ZAdd(ctx, "test:key", queue.ScoredMember{Member: "out1_0", Score: 100})
	mockQueue.ZAdd(ctx, "test:key", queue.ScoredMember{Member: "out2_0", Score: 200})
	mockQueue.ZAdd(ctx, "test:key", queue.ScoredMember{Member: "out3_0", Score: 300})

	// Mark out3 as spent
	mockQueue.HSet(ctx, SpendsKey, "out3_0", "spendingtx")

	balance, err := store.SearchBalance(ctx, &SearchCfg{
		Keys: []string{"test:key"},
	})
	require.NoError(t, err)
	assert.Equal(t, uint64(300), balance) // 100 + 200, excluding spent
}

func TestCountMembers(t *testing.T) {
	store, mockQueue, ctx := setupSearchStore(t)

	mockQueue.ZAdd(ctx, "test:key", queue.ScoredMember{Member: "out1_0", Score: 100})
	mockQueue.ZAdd(ctx, "test:key", queue.ScoredMember{Member: "out2_0", Score: 200})
	mockQueue.ZAdd(ctx, "test:key", queue.ScoredMember{Member: "out3_0", Score: 300})

	count, err := store.CountMembers(ctx, "test:key")
	require.NoError(t, err)
	assert.Equal(t, uint64(3), count)
}

func TestCountMembers_Empty(t *testing.T) {
	store, _, ctx := setupSearchStore(t)

	count, err := store.CountMembers(ctx, "nonexistent:key")
	require.NoError(t, err)
	assert.Equal(t, uint64(0), count)
}
