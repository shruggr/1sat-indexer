# 1sat-indexer v6 Testing Plan

## Overview

This document outlines the testing and benchmarking strategy for the refactored 1sat-indexer. The v6 refactor introduced a unified storage model using overlay's `QueueStorage`, stream-based event architecture, and simplified IndexContext.

---

## Test Directory Structure

```
1sat-indexer/
├── idx/
│   ├── store_test.go       # QueueStore unit tests
│   ├── search_test.go      # Search logic tests
│   └── context_test.go     # IndexContext/HeightScore tests
├── mod/
│   ├── onesat/
│   │   ├── origin_test.go
│   │   ├── inscription_test.go
│   │   ├── bsv20_test.go
│   │   ├── bsv21_test.go
│   │   └── ordlock_test.go
│   └── bitcom/
│       ├── map_test.go
│       └── sigma_test.go
├── ingest/
│   └── ingest_test.go      # Ingestion pipeline tests
└── benchmark/
    └── bench_test.go       # Performance benchmarks
```

---

## Priority 1: Core Storage Tests

**File**: `idx/store_test.go`

### SaveTxos Tests

| Test Name | Description | Verifications |
|-----------|-------------|---------------|
| `TestSaveTxos_SingleOutput` | Save transaction with one output | TxoKey hash populated, SatsKey set, events indexed |
| `TestSaveTxos_MultipleOutputs` | Save transaction with 10 outputs | All outpoints indexed, txid event contains all |
| `TestSaveTxos_WithProtocolData` | Save with inscription/BSV20 data | TdataKey populated for tag, tag event indexed |
| `TestSaveTxos_WithOwners` | Save P2PKH outputs | `own:{address}` events created |
| `TestSaveTxos_EventPublishing` | Verify PubSub calls | Each event key published with outpoint |

### LoadTxo Tests

| Test Name | Description | Verifications |
|-----------|-------------|---------------|
| `TestLoadTxo_Basic` | Load single TXO by outpoint | Height, Idx, Satoshis, Owners populated |
| `TestLoadTxo_WithTags` | Load with specific tag data | Data map contains requested tags only |
| `TestLoadTxo_WithSpend` | Load with spend flag | Spend field populated if spent |
| `TestLoadTxo_NotFound` | Load non-existent outpoint | Returns nil, no error |
| `TestLoadTxos_Batch` | Batch load 100 outpoints | Correct TXOs returned, nils for missing |
| `TestLoadTxosByTxid` | Load all outputs for txid | All vouts returned in order |

### SaveSpends Tests

| Test Name | Description | Verifications |
|-----------|-------------|---------------|
| `TestSaveSpends_SingleInput` | Record one spend | SpendsKey set, `own:{addr}:spnd` event created |
| `TestSaveSpends_MultipleInputs` | Record 5 spends | All spends recorded, events for each owner |
| `TestSaveSpends_EventsAppended` | Spend appends to existing events | EventsKey JSON array extended |

### Rollback Tests

| Test Name | Description | Verifications |
|-----------|-------------|---------------|
| `TestRollback_RemovesAllEvents` | Rollback transaction | All ZSets cleaned, hashes deleted |
| `TestRollback_PreservesOtherTxos` | Rollback doesn't affect other txs | Unrelated TXOs remain |
| `TestRollback_NonExistentTx` | Rollback unknown txid | No error, no-op |

### Edge Cases

| Test Name | Description |
|-----------|-------------|
| `TestSetNewSpend_Idempotent` | Second SetNewSpend returns false |
| `TestSaveTxos_ZeroSatoshis` | Handle 0-sat outputs correctly |
| `TestSaveTxos_EmptyOwners` | TXO with no identifiable owner |

---

## Priority 2: Search Tests

**File**: `idx/search_test.go`

### Basic Search

| Test Name | Description | Verifications |
|-----------|-------------|---------------|
| `TestSearch_SingleKey` | Query one sorted set | Results in score order |
| `TestSearch_EmptyKey` | Query empty/non-existent key | Empty slice, no error |
| `TestSearch_DefaultLimit` | No limit specified | Returns up to 1000 |
| `TestSearch_CustomLimit` | Limit=50 | Exactly 50 results max |

### Multi-Key Search

| Test Name | Description | Verifications |
|-----------|-------------|---------------|
| `TestSearch_MultiKey_OR` | Union of 3 keys | Contains members from all keys |
| `TestSearch_MultiKey_AND` | Intersection of 3 keys | Only members in ALL keys |
| `TestSearch_AND_NoOverlap` | AND with disjoint sets | Empty result |
| `TestSearch_OR_Deduplication` | Same member in multiple keys | Appears once in results |

### Filtering & Pagination

| Test Name | Description | Verifications |
|-----------|-------------|---------------|
| `TestSearch_FilterSpent` | Exclude spent outputs | Only unspent in results |
| `TestSearch_FilterSpent_AllSpent` | All candidates spent | Empty result |
| `TestSearch_ScoreRange_From` | From score specified | No results below From |
| `TestSearch_ScoreRange_To` | To score specified | No results above To |
| `TestSearch_ScoreRange_Both` | From and To | Results within range |
| `TestSearch_Pagination` | Page through large set | Consistent ordering, no duplicates |
| `TestSearch_Reverse` | Descending order | Highest scores first |

### SearchTxos & SearchBalance

| Test Name | Description | Verifications |
|-----------|-------------|---------------|
| `TestSearchTxos_LoadsData` | Search and load TXO data | Full Txo structs returned |
| `TestSearchBalance_SumsUnspent` | Calculate balance | Correct sum of unspent sats |
| `TestSearchBalance_ExcludesSpent` | Spent outputs excluded | Balance excludes spent |

---

## Priority 3: IndexContext Tests

**File**: `idx/context_test.go`

| Test Name | Description | Verifications |
|-----------|-------------|---------------|
| `TestHeightScore_BlockTx` | Block transaction scoring | `height * 1e9 + blockIndex` |
| `TestHeightScore_MempoolTx` | Mempool transaction | Negative score (timestamp-based) |
| `TestHeightScore_Ordering` | Score ordering correctness | Earlier blocks < later blocks |
| `TestTxo_AddOwner_Deduplication` | Add same owner twice | Only one entry |

---

## Priority 4: Protocol Indexer Tests

### Origin Indexer (`mod/onesat/origin_test.go`)

| Test Name | Description |
|-----------|-------------|
| `TestOrigin_BelowTriggerHeight` | No indexing before block 783968 |
| `TestOrigin_NonOneSat` | Outputs > 1 sat not indexed |
| `TestOrigin_FirstInscription` | Creates new origin from inscription |
| `TestOrigin_InheritedOrigin` | Tracks origin through spends |
| `TestOrigin_NonceIncrement` | Nonce increments on inheritance |
| `TestOrigin_ParentEvent` | Parent outpoint event generated |

### Inscription Indexer (`mod/onesat/inscription_test.go`)

| Test Name | Description |
|-----------|-------------|
| `TestInscription_TextContent` | Parse plain text inscription |
| `TestInscription_JSONContent` | Parse JSON, classify correctly |
| `TestInscription_BinaryContent` | Handle binary (image, etc.) |
| `TestInscription_WithParent` | Extract parent pointer |
| `TestInscription_ContentType` | Content-type field extraction |
| `TestInscription_MalformedScript` | Graceful handling of bad data |
| `TestInscription_OwnerExtraction` | P2PKH after inscription |

### BSV20 Indexer (`mod/onesat/bsv20_test.go`)

| Test Name | Description |
|-----------|-------------|
| `TestBSV20_Deploy` | Parse deploy operation |
| `TestBSV20_Mint` | Parse mint operation |
| `TestBSV20_Transfer` | Parse transfer operation |
| `TestBSV20_TickerNormalization` | Uppercase, max 4 chars |
| `TestBSV20_InvalidJSON` | Reject malformed JSON |
| `TestBSV20_DecimalValidation` | Decimals 0-18 only |

### BSV21 Indexer (`mod/onesat/bsv21_test.go`)

| Test Name | Description |
|-----------|-------------|
| `TestBSV21_Deploy` | Token deployment parsing |
| `TestBSV21_Mint` | Mint with icon/metadata |
| `TestBSV21_Transfer_Valid` | Balance tracking, valid state |
| `TestBSV21_Transfer_Invalid` | Insufficient balance → invalid |
| `TestBSV21_PreSave_StateTransition` | Pending → valid/invalid logic |
| `TestBSV21_ReasonMessage` | Correct reason for invalid |

### OrdLock Indexer (`mod/onesat/ordlock_test.go`)

| Test Name | Description |
|-----------|-------------|
| `TestOrdLock_ScriptDetection` | Identify OrdLock script pattern |
| `TestOrdLock_PriceExtraction` | Extract listing price |
| `TestOrdLock_PricePerCalculation` | Price-per for token listings |
| `TestOrdLock_PreSave_Sale` | Detect sale (spend with payment) |
| `TestOrdLock_PreSave_Cancel` | Detect cancel (owner reclaim) |

### MAP Indexer (`mod/bitcom/map_test.go`)

| Test Name | Description |
|-----------|-------------|
| `TestMAP_BasicKV` | Parse key-value pairs |
| `TestMAP_SET_Operation` | SET operation handling |
| `TestMAP_Merge` | Merge with inherited MAP data |

### SIGMA Indexer (`mod/bitcom/sigma_test.go`)

| Test Name | Description |
|-----------|-------------|
| `TestSIGMA_ValidSignature` | Verify valid signature |
| `TestSIGMA_InvalidSignature` | Detect invalid signature |
| `TestSIGMA_MessageHash` | Correct hash computation |

---

## Priority 5: Ingestion Pipeline Tests

**File**: `ingest/ingest_test.go`

| Test Name | Description |
|-----------|-------------|
| `TestIngestTxid_Simple` | Ingest by txid, verify storage |
| `TestIngestTxid_WithInscription` | Full inscription flow |
| `TestIngestTx_Spends` | Spend tracking on ingest |
| `TestIngestTx_ScoreCalculation` | Correct score for block/mempool |

---

## Benchmarks

**File**: `benchmark/bench_test.go`

### Storage Benchmarks

```go
func BenchmarkSaveTxos_1Output(b *testing.B)
func BenchmarkSaveTxos_10Outputs(b *testing.B)
func BenchmarkSaveTxos_100Outputs(b *testing.B)
func BenchmarkLoadTxo_Single(b *testing.B)
func BenchmarkLoadTxos_Batch100(b *testing.B)
func BenchmarkSaveSpends_10Inputs(b *testing.B)
```

### Search Benchmarks

```go
func BenchmarkSearch_1Key_100Results(b *testing.B)
func BenchmarkSearch_3Keys_OR_100Results(b *testing.B)
func BenchmarkSearch_3Keys_AND_100Results(b *testing.B)
func BenchmarkSearch_FilterSpent_1000Candidates(b *testing.B)
func BenchmarkSearchBalance_1000Utxos(b *testing.B)
```

### End-to-End Benchmarks

```go
func BenchmarkIngestTransaction_Simple(b *testing.B)
func BenchmarkIngestTransaction_WithInscription(b *testing.B)
func BenchmarkIngestTransaction_WithBSV21(b *testing.B)
```

### Metrics to Capture

For each benchmark:
- `ns/op` - Time per operation
- `B/op` - Bytes allocated per operation
- `allocs/op` - Allocations per operation

---

## Mock Strategy

### MockQueueStorage

For unit tests, implement `queue.QueueStorage` interface with in-memory maps:

```go
type MockQueueStorage struct {
    hashes   map[string]map[string]string  // key -> field -> value
    zsets    map[string][]ScoredMember     // key -> sorted members
}

func (m *MockQueueStorage) HSet(ctx context.Context, key, field, value string) error
func (m *MockQueueStorage) HGet(ctx context.Context, key, field string) (string, error)
func (m *MockQueueStorage) HMGet(ctx context.Context, key string, fields ...string) ([]string, error)
func (m *MockQueueStorage) ZAdd(ctx context.Context, key string, members ...ScoredMember) error
func (m *MockQueueStorage) ZRange(ctx context.Context, key string, r ScoreRange) ([]ScoredMember, error)
// ... etc
```

### MockPubSub

Track published events for verification:

```go
type MockPubSub struct {
    published []PublishCall
}

type PublishCall struct {
    Channel string
    Message string
}
```

### Integration Tests

For integration tests requiring real Redis:
- Use environment variable `REDIS_URL` or skip if not available
- Tag with `//go:build integration`
- Run separately: `go test -tags=integration ./...`

---

## Test Fixtures

### Transaction Fixtures Needed

To build comprehensive test fixtures, example transactions are needed for each protocol type. These will be fetched and converted to static test data.

| Protocol | Type | Required Fields |
|----------|------|-----------------|
| Origin | First inscription | txid, vout, content |
| Origin | Inherited | txid, parent origin |
| Inscription | Text | txid, content-type, content |
| Inscription | JSON | txid, parsed fields |
| Inscription | Image | txid, content-type, binary hash |
| BSV20 | Deploy | txid, tick, max, lim, dec |
| BSV20 | Mint | txid, tick, amt |
| BSV20 | Transfer | txid, tick, amt |
| BSV21 | Deploy | txid, sym, icon, amt, dec |
| BSV21 | Transfer (valid) | txid, token inputs, outputs |
| BSV21 | Transfer (invalid) | txid, insufficient inputs |
| OrdLock | Listed | txid, price, locked outpoint |
| OrdLock | Sold | txid, spend details |
| MAP | SET | txid, key-value pairs |
| SIGMA | Valid sig | txid, signature, signer |

---

## Questions Requiring Answers

### 1. Example Transactions

Can you provide 2-3 example txids for each transaction type listed above? I'll fetch them via WhatsOnChain/JungleBus to create static test fixtures.

**Needed:**
- [ ] Origin (first inscription creating an origin)
- [ ] Origin (inherited through spend)
- [ ] Inscription (text)
- [ ] Inscription (JSON metadata)
- [ ] Inscription (image/binary)
- [ ] BSV20 deploy
- [ ] BSV20 mint
- [ ] BSV21 deploy
- [ ] BSV21 transfer (valid)
- [ ] BSV21 transfer (invalid/insufficient)
- [ ] OrdLock listing
- [ ] OrdLock sale
- [ ] MAP metadata
- [ ] SIGMA signed message

### 2. Mock vs Integration Test Preference

**Option A: Mock-first**
- Unit tests use MockQueueStorage (fast, isolated)
- Separate integration tests with real Redis (tagged)
- Benchmarks can run either way

**Option B: Integration-first**
- All tests require Redis connection
- More realistic but slower
- CI requires Redis service

**Recommendation**: Option A (mock-first) for faster iteration. What's your preference?

### 3. Benchmark Target Environment

Should benchmarks measure:
- **A)** Pure Go operations with mock storage (measures code efficiency)
- **B)** Real Redis operations (measures actual throughput)
- **C)** Both (separate benchmark suites)

**Recommendation**: C (both) - mock for code optimization, Redis for realistic throughput. Thoughts?

### 4. Test Coverage Targets

What level of coverage are you targeting?
- Critical paths only (~60%)
- Comprehensive (~80%)
- Exhaustive (~90%+)

### 5. CI Integration

Will these tests run in CI? If so:
- Do you have Redis available in CI?
- Any timeout constraints?
- Parallel test execution?

---

## Implementation Order

1. **MockQueueStorage & MockPubSub** - Foundation for all tests
2. **idx/store_test.go** - Core storage operations
3. **idx/search_test.go** - Search logic
4. **Protocol indexer tests** - One at a time, starting with Origin
5. **Benchmarks** - After functional tests pass
6. **Integration tests** - If Redis available

---

## Running Tests

```bash
# Unit tests (mock storage)
go test ./idx/... ./mod/...

# Integration tests (requires Redis)
go test -tags=integration ./...

# Benchmarks
go test -bench=. -benchmem ./benchmark/...

# Coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```
