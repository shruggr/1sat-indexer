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

func newTestStore() (*QueueStore, *MockQueueStorage, *MockPubSub) {
	mockQueue := NewMockQueueStorage()
	mockPubSub := NewMockPubSub()
	store := NewQueueStore(mockQueue, mockPubSub)
	return store, mockQueue, mockPubSub
}

func newTestOutpoint(txid string, vout uint32) *lib.Outpoint {
	op, _ := lib.NewOutpointFromString(txid + "_" + string(rune('0'+vout)))
	return op
}

func TestSaveTxos_SingleOutput(t *testing.T) {
	store, mockQueue, mockPubSub := newTestStore()
	ctx := context.Background()

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
				Owners:   []string{"1PKH123..."},
				Data:     make(map[string]*IndexData),
			},
		},
	}

	err := store.SaveTxos(idxCtx)
	require.NoError(t, err)

	// Verify TXO was stored
	txoJSON, err := mockQueue.HGet(ctx, TxoKey, outpoint.String())
	require.NoError(t, err)
	assert.NotEmpty(t, txoJSON)

	var stored storedTxo
	err = json.Unmarshal([]byte(txoJSON), &stored)
	require.NoError(t, err)
	assert.Equal(t, uint32(800000), stored.Height)
	assert.Equal(t, uint64(0), stored.Idx)
	assert.Equal(t, uint64(1000), stored.Satoshis)
	assert.Equal(t, []string{"1PKH123..."}, stored.Owners)

	// Verify sats stored
	satsStr, _ := mockQueue.HGet(ctx, SatsKey, outpoint.String())
	assert.Equal(t, "1000", satsStr)

	// Verify events sorted set created
	members, _ := mockQueue.ZRange(ctx, "txid:"+txid, queue.ScoreRange{})
	assert.Len(t, members, 1)
	assert.Equal(t, outpoint.String(), members[0].Member)

	// Verify owner sorted set created
	ownerMembers, _ := mockQueue.ZRange(ctx, "own:1PKH123...", queue.ScoreRange{})
	assert.Len(t, ownerMembers, 1)

	// Verify pubsub events published
	published := mockPubSub.GetPublished()
	assert.NotEmpty(t, published)
}

func TestSaveTxos_MultipleOutputs(t *testing.T) {
	store, mockQueue, _ := newTestStore()
	ctx := context.Background()

	txid := "0000000000000000000000000000000000000000000000000000000000000002"
	outpoint0, _ := lib.NewOutpointFromString(txid + "_0")
	outpoint1, _ := lib.NewOutpointFromString(txid + "_1")
	sats0 := uint64(500)
	sats1 := uint64(1500)

	idxCtx := &IndexContext{
		Ctx:     ctx,
		TxidHex: txid,
		Score:   800001000000.0,
		Txos: []*Txo{
			{
				Outpoint: outpoint0,
				Height:   800001,
				Idx:      0,
				Satoshis: &sats0,
				Owners:   []string{"addr1"},
				Data:     make(map[string]*IndexData),
			},
			{
				Outpoint: outpoint1,
				Height:   800001,
				Idx:      0,
				Satoshis: &sats1,
				Owners:   []string{"addr2"},
				Data:     make(map[string]*IndexData),
			},
		},
	}

	err := store.SaveTxos(idxCtx)
	require.NoError(t, err)

	// Both outputs should be stored
	txoJSON0, _ := mockQueue.HGet(ctx, TxoKey, outpoint0.String())
	txoJSON1, _ := mockQueue.HGet(ctx, TxoKey, outpoint1.String())
	assert.NotEmpty(t, txoJSON0)
	assert.NotEmpty(t, txoJSON1)

	// Both should be in txid sorted set
	members, _ := mockQueue.ZRange(ctx, "txid:"+txid, queue.ScoreRange{})
	assert.Len(t, members, 2)
}

func TestSaveTxos_WithProtocolData(t *testing.T) {
	store, mockQueue, _ := newTestStore()
	ctx := context.Background()

	txid := "0000000000000000000000000000000000000000000000000000000000000003"
	outpoint, _ := lib.NewOutpointFromString(txid + "_0")
	sats := uint64(1)

	idxCtx := &IndexContext{
		Ctx:     ctx,
		TxidHex: txid,
		Score:   800002000000.0,
		Txos: []*Txo{
			{
				Outpoint: outpoint,
				Height:   800002,
				Idx:      0,
				Satoshis: &sats,
				Owners:   []string{"addr1"},
				Data: map[string]*IndexData{
					"ord": {
						Data: map[string]interface{}{"type": "text/plain"},
						Events: []*Event{
							{Id: "type", Value: "text/plain"},
						},
					},
				},
			},
		},
	}

	err := store.SaveTxos(idxCtx)
	require.NoError(t, err)

	// Verify tag data stored
	tagData, _ := mockQueue.HGet(ctx, TdataKey, outpoint.String()+":ord")
	assert.NotEmpty(t, tagData)

	// Verify tag sorted set created
	tagMembers, _ := mockQueue.ZRange(ctx, "ord", queue.ScoreRange{})
	assert.Len(t, tagMembers, 1)

	// Verify event sorted set created
	eventMembers, _ := mockQueue.ZRange(ctx, "ord:type:text/plain", queue.ScoreRange{})
	assert.Len(t, eventMembers, 1)
}

func TestLoadTxo_Basic(t *testing.T) {
	store, mockQueue, _ := newTestStore()
	ctx := context.Background()

	txid := "0000000000000000000000000000000000000000000000000000000000000004"
	outpoint, _ := lib.NewOutpointFromString(txid + "_0")

	// Pre-populate store
	stored := storedTxo{
		Height:   800003,
		Idx:      5,
		Satoshis: 2000,
		Owners:   []string{"owner1", "owner2"},
	}
	storedJSON, _ := json.Marshal(stored)
	mockQueue.HSet(ctx, TxoKey, outpoint.String(), string(storedJSON))

	// Load it back
	txo, err := store.LoadTxo(ctx, outpoint.String(), nil, false)
	require.NoError(t, err)
	require.NotNil(t, txo)

	assert.Equal(t, uint32(800003), txo.Height)
	assert.Equal(t, uint64(5), txo.Idx)
	assert.Equal(t, uint64(2000), *txo.Satoshis)
	assert.Equal(t, []string{"owner1", "owner2"}, txo.Owners)
}

func TestLoadTxo_NotFound(t *testing.T) {
	store, _, _ := newTestStore()
	ctx := context.Background()

	txo, err := store.LoadTxo(ctx, "nonexistent_0", nil, false)
	require.NoError(t, err)
	assert.Nil(t, txo)
}

func TestLoadTxo_WithTags(t *testing.T) {
	store, mockQueue, _ := newTestStore()
	ctx := context.Background()

	txid := "0000000000000000000000000000000000000000000000000000000000000005"
	outpoint, _ := lib.NewOutpointFromString(txid + "_0")

	// Pre-populate TXO
	stored := storedTxo{Height: 800004, Idx: 0, Satoshis: 1}
	storedJSON, _ := json.Marshal(stored)
	mockQueue.HSet(ctx, TxoKey, outpoint.String(), string(storedJSON))

	// Pre-populate tag data
	tagData := `{"type":"image/png","size":1024}`
	mockQueue.HSet(ctx, TdataKey, outpoint.String()+":ord", tagData)

	// Load with tags
	txo, err := store.LoadTxo(ctx, outpoint.String(), []string{"ord"}, false)
	require.NoError(t, err)
	require.NotNil(t, txo)
	require.NotNil(t, txo.Data["ord"])
}

func TestLoadTxo_WithSpend(t *testing.T) {
	store, mockQueue, _ := newTestStore()
	ctx := context.Background()

	txid := "0000000000000000000000000000000000000000000000000000000000000006"
	outpoint, _ := lib.NewOutpointFromString(txid + "_0")
	spendTxid := "0000000000000000000000000000000000000000000000000000000000000007"

	// Pre-populate TXO
	stored := storedTxo{Height: 800005, Idx: 0, Satoshis: 1}
	storedJSON, _ := json.Marshal(stored)
	mockQueue.HSet(ctx, TxoKey, outpoint.String(), string(storedJSON))

	// Pre-populate spend
	mockQueue.HSet(ctx, SpendsKey, outpoint.String(), spendTxid)

	// Load with spend
	txo, err := store.LoadTxo(ctx, outpoint.String(), nil, true)
	require.NoError(t, err)
	require.NotNil(t, txo)
	assert.Equal(t, spendTxid, txo.Spend)
}

func TestLoadTxos_Batch(t *testing.T) {
	store, mockQueue, _ := newTestStore()
	ctx := context.Background()

	txid := "0000000000000000000000000000000000000000000000000000000000000008"
	outpoint0, _ := lib.NewOutpointFromString(txid + "_0")
	outpoint1, _ := lib.NewOutpointFromString(txid + "_1")

	// Pre-populate
	stored0 := storedTxo{Height: 800006, Idx: 0, Satoshis: 100}
	stored1 := storedTxo{Height: 800006, Idx: 0, Satoshis: 200}
	json0, _ := json.Marshal(stored0)
	json1, _ := json.Marshal(stored1)
	mockQueue.HSet(ctx, TxoKey, outpoint0.String(), string(json0))
	mockQueue.HSet(ctx, TxoKey, outpoint1.String(), string(json1))

	// Load batch
	txos, err := store.LoadTxos(ctx, []string{outpoint0.String(), outpoint1.String()}, nil, false)
	require.NoError(t, err)
	require.Len(t, txos, 2)
	assert.Equal(t, uint64(100), *txos[0].Satoshis)
	assert.Equal(t, uint64(200), *txos[1].Satoshis)
}

func TestLoadTxos_EmptyInput(t *testing.T) {
	store, _, _ := newTestStore()
	ctx := context.Background()

	txos, err := store.LoadTxos(ctx, []string{}, nil, false)
	require.NoError(t, err)
	assert.Nil(t, txos)
}

func TestSaveSpends(t *testing.T) {
	store, mockQueue, mockPubSub := newTestStore()
	ctx := context.Background()

	txid := "0000000000000000000000000000000000000000000000000000000000000009"
	spendTxid := "000000000000000000000000000000000000000000000000000000000000000a"
	outpoint, _ := lib.NewOutpointFromString(txid + "_0")
	sats := uint64(1000)

	// Pre-populate the TXO being spent
	mockQueue.HSet(ctx, EventsKey, outpoint.String(), `["txid:`+txid+`","own:owner1"]`)

	idxCtx := &IndexContext{
		Ctx:     ctx,
		TxidHex: spendTxid,
		Score:   800007000000.0,
		Spends: []*Txo{
			{
				Outpoint: outpoint,
				Satoshis: &sats,
				Owners:   []string{"owner1"},
			},
		},
	}

	err := store.SaveSpends(idxCtx)
	require.NoError(t, err)

	// Verify spend recorded
	spend, _ := mockQueue.HGet(ctx, SpendsKey, outpoint.String())
	assert.Equal(t, spendTxid, spend)

	// Verify owner spent sorted set
	spentMembers, _ := mockQueue.ZRange(ctx, "own:owner1:spnd", queue.ScoreRange{})
	assert.Len(t, spentMembers, 1)

	// Verify pubsub event
	published := mockPubSub.GetPublished()
	assert.NotEmpty(t, published)
}

func TestGetSpend(t *testing.T) {
	store, mockQueue, _ := newTestStore()
	ctx := context.Background()

	outpoint := "abc123_0"
	mockQueue.HSet(ctx, SpendsKey, outpoint, "spendtxid")

	spend, err := store.GetSpend(ctx, outpoint)
	require.NoError(t, err)
	assert.Equal(t, "spendtxid", spend)
}

func TestGetSpend_NotFound(t *testing.T) {
	store, _, _ := newTestStore()
	ctx := context.Background()

	spend, err := store.GetSpend(ctx, "nonexistent_0")
	require.NoError(t, err)
	assert.Empty(t, spend)
}

func TestSetNewSpend(t *testing.T) {
	store, mockQueue, _ := newTestStore()
	ctx := context.Background()

	outpoint := "abc123_0"

	// First set should succeed
	set, err := store.SetNewSpend(ctx, outpoint, "txid1")
	require.NoError(t, err)
	assert.True(t, set)

	// Second set should fail (already spent)
	set, err = store.SetNewSpend(ctx, outpoint, "txid2")
	require.NoError(t, err)
	assert.False(t, set)

	// Original spend should be preserved
	spend, _ := mockQueue.HGet(ctx, SpendsKey, outpoint)
	assert.Equal(t, "txid1", spend)
}

func TestUnsetSpends(t *testing.T) {
	store, mockQueue, _ := newTestStore()
	ctx := context.Background()

	outpoints := []string{"out1_0", "out2_0"}
	mockQueue.HSet(ctx, SpendsKey, "out1_0", "tx1")
	mockQueue.HSet(ctx, SpendsKey, "out2_0", "tx2")

	err := store.UnsetSpends(ctx, outpoints)
	require.NoError(t, err)

	spend1, _ := mockQueue.HGet(ctx, SpendsKey, "out1_0")
	spend2, _ := mockQueue.HGet(ctx, SpendsKey, "out2_0")
	assert.Empty(t, spend1)
	assert.Empty(t, spend2)
}

func TestRollback(t *testing.T) {
	store, mockQueue, _ := newTestStore()
	ctx := context.Background()

	txid := "000000000000000000000000000000000000000000000000000000000000000b"
	outpoint, _ := lib.NewOutpointFromString(txid + "_0")
	sats := uint64(1000)

	// First save a TXO
	idxCtx := &IndexContext{
		Ctx:     ctx,
		TxidHex: txid,
		Score:   800008000000.0,
		Txos: []*Txo{
			{
				Outpoint: outpoint,
				Height:   800008,
				Idx:      0,
				Satoshis: &sats,
				Owners:   []string{"owner1"},
				Data:     make(map[string]*IndexData),
			},
		},
	}

	err := store.SaveTxos(idxCtx)
	require.NoError(t, err)

	// Verify it exists
	txoJSON, _ := mockQueue.HGet(ctx, TxoKey, outpoint.String())
	assert.NotEmpty(t, txoJSON)

	// Now rollback
	err = store.Rollback(ctx, txid)
	require.NoError(t, err)

	// Verify TXO removed
	txoJSON, _ = mockQueue.HGet(ctx, TxoKey, outpoint.String())
	assert.Empty(t, txoJSON)

	// Verify events removed
	members, _ := mockQueue.ZRange(ctx, "txid:"+txid, queue.ScoreRange{})
	assert.Empty(t, members)
}

func TestGetSats(t *testing.T) {
	store, mockQueue, _ := newTestStore()
	ctx := context.Background()

	mockQueue.HSet(ctx, SatsKey, "out1_0", "100")
	mockQueue.HSet(ctx, SatsKey, "out2_0", "200")

	sats, err := store.GetSats(ctx, []string{"out1_0", "out2_0", "out3_0"})
	require.NoError(t, err)
	require.Len(t, sats, 3)
	assert.Equal(t, uint64(100), sats[0])
	assert.Equal(t, uint64(200), sats[1])
	assert.Equal(t, uint64(0), sats[2]) // missing returns 0
}

func TestLog(t *testing.T) {
	store, mockQueue, _ := newTestStore()
	ctx := context.Background()

	err := store.Log(ctx, "test:log", "item1", 100.0)
	require.NoError(t, err)

	members, _ := mockQueue.ZRange(ctx, "test:log", queue.ScoreRange{})
	require.Len(t, members, 1)
	assert.Equal(t, "item1", members[0].Member)
	assert.Equal(t, 100.0, members[0].Score)
}

func TestLogOnce(t *testing.T) {
	store, _, _ := newTestStore()
	ctx := context.Background()

	// First log should succeed
	added, err := store.LogOnce(ctx, "test:once", "item1", 100.0)
	require.NoError(t, err)
	assert.True(t, added)

	// Second log should fail (already exists)
	added, err = store.LogOnce(ctx, "test:once", "item1", 200.0)
	require.NoError(t, err)
	assert.False(t, added)
}

func TestLogScore(t *testing.T) {
	store, _, _ := newTestStore()
	ctx := context.Background()

	store.Log(ctx, "test:score", "item1", 150.0)

	score, err := store.LogScore(ctx, "test:score", "item1")
	require.NoError(t, err)
	assert.Equal(t, 150.0, score)
}

func TestLogScore_NotFound(t *testing.T) {
	store, _, _ := newTestStore()
	ctx := context.Background()

	score, err := store.LogScore(ctx, "test:score", "nonexistent")
	require.NoError(t, err)
	assert.Equal(t, 0.0, score)
}

func TestDelog(t *testing.T) {
	store, mockQueue, _ := newTestStore()
	ctx := context.Background()

	store.Log(ctx, "test:delog", "item1", 100.0)
	store.Log(ctx, "test:delog", "item2", 200.0)

	err := store.Delog(ctx, "test:delog", "item1")
	require.NoError(t, err)

	members, _ := mockQueue.ZRange(ctx, "test:delog", queue.ScoreRange{})
	require.Len(t, members, 1)
	assert.Equal(t, "item2", members[0].Member)
}

func TestLoadTxosByTxid(t *testing.T) {
	store, mockQueue, _ := newTestStore()
	ctx := context.Background()

	txid := "000000000000000000000000000000000000000000000000000000000000000c"
	outpoint0, _ := lib.NewOutpointFromString(txid + "_0")
	outpoint1, _ := lib.NewOutpointFromString(txid + "_1")

	// Pre-populate TXOs
	stored0 := storedTxo{Height: 800009, Idx: 0, Satoshis: 100}
	stored1 := storedTxo{Height: 800009, Idx: 0, Satoshis: 200}
	json0, _ := json.Marshal(stored0)
	json1, _ := json.Marshal(stored1)
	mockQueue.HSet(ctx, TxoKey, outpoint0.String(), string(json0))
	mockQueue.HSet(ctx, TxoKey, outpoint1.String(), string(json1))

	// Pre-populate txid sorted set
	mockQueue.ZAdd(ctx, "txid:"+txid, queue.ScoredMember{Member: outpoint0.String(), Score: 800009000000.0})
	mockQueue.ZAdd(ctx, "txid:"+txid, queue.ScoredMember{Member: outpoint1.String(), Score: 800009000000.0})

	// Load by txid
	txos, err := store.LoadTxosByTxid(ctx, txid, nil, false)
	require.NoError(t, err)
	require.Len(t, txos, 2)
}
