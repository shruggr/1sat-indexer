# 1sat-indexer v6 Optimization Plan

## Overview
Transform 1sat-indexer from relational database-centric to stream-based architecture, eliminating database read bottlenecks and enabling efficient UTXO tracking through dual-event streams. **Leverage overlay package's `QueueStorage` for all storage - txos stored as hashes, events stored as sorted sets.**

---

## Architectural Decisions

### 1. **Dual-Event Stream Architecture** (REVISED)
- **Separate event keys for unspent vs spent** (NOT prefixes within a single key)
- **Unspent events**: `own:{address}` - outpoint logged when TXO created
- **Spent events**: `osp:{address}` (owner spent) - outpoint logged when TXO spent
- Events stored as sorted sets via `QueueStorage.ZAdd()` from overlay package
- Query via `QueueStorage.ZRange()` with score-based pagination

**Example data storage**:
```
# TXO stored as hash
txo:abc123_0 -> {
  height: "800000",
  idx: "0",
  satoshis: "1000",
  owners: '["1PKH..."]',
  events: '["txid:abc123", "ord", "ord:origin:xyz_0", "own:1PKH..."]',
  spend: "",
  ord: '{"origin": "xyz_0", ...}'   # tag-specific data
}

# Events stored as sorted sets (score = height * 1e6 + idx)
own:1PKH... -> [{member: "abc123_0", score: 800000000000.0}]   // unspent
osp:1PKH... -> [{member: "abc123_0", score: 800001500000.0}]   // spent
txid:abc123 -> [{member: "abc123_0", score: 800000000000.0}]   // lookup by txid
ord -> [{member: "abc123_0", score: 800000000000.0}]           // tag index
```

### 2. **Use overlay's QueueStorage**
- Replace custom database implementations with `QueueStorage` interface
- Redis-like abstraction supporting multiple backends (Redis, SQLite, MongoDB, Postgres)
- Operations:
  - **Hash ops**: `HSet`, `HGet`, `HGetAll`, `HDel`, `HMSet`, `HMGet` - for txo data
  - **Sorted set ops**: `ZAdd`, `ZRem`, `ZRange`, `ZScore`, `ZCard` - for event indexes
  - **Set ops**: `SAdd`, `SMembers`, `SRem`, `SIsMember` - for general sets
- Score-based pagination via `ZRange` with `ScoreRange{Min, Max, Count}`

### 3. **Mandatory Ancestor Re-parsing**
- Transactions arrive with `SourceTransaction` populated on each input
- `ParseSpends()` creates IndexContext for each parent tx and parses outputs
- Database becomes **write-mostly derived view**, not source of truth during indexing
- Source transactions are **required** for proper indexing (hard requirement)

### 4. **Remove Account Abstraction** ✅ COMPLETED
- Accounts mapped multiple owner addresses to a single identifier (server-side convenience)
- This creates DB read dependencies during indexing (`AcctOwners()` lookups)
- **V6 approach**: Clients subscribe to multiple `own:{address}` streams and merge client-side

### 5. **API Strategy**
- **Priority**: New stream-focused endpoints using `QueueStorage`
  - `/api/v2/events/{event}` - Paginated event lookup via `ZRange`
  - `/api/v2/owner/{address}` - Query `own:` and `osp:` events
  - SSE via overlay's `SSEManager`
- **Legacy APIs**: De-prioritized, updated to use QueueStore under the hood

### 6. **Storage Layer**
- **Primary**: overlay's `QueueStorage` with Redis, SQLite, MongoDB, or Postgres backends
- **Data model**: TXOs as hashes, events as sorted sets
- **Shared services**: BEEF storage, PubSub, QueueStorage from overlay

### 7. **Migration Strategy**
- v6 requires **fresh re-index from genesis** (no migration tools)
- Clean slate ensures all data has dual-event format
- Users can run v5/v6 side-by-side during transition

---

## Integration with overlay Package

### Components Adopted from `github.com/b-open-io/overlay`:

#### **1. BEEF Storage** ✅ INTEGRATED
- Chainable storage layers: `LRU → Redis/SQLite → JungleBus`
- `config.BeefStorage` used for transaction loading

#### **2. Pub/Sub Abstraction** ✅ INTEGRATED
- `pubsub.PubSub` interface from overlay package
- Supports Redis (`redis://`) or in-memory channels (`channels://`)
- `config.PubSub` initialized via `pubsub.CreatePubSub()`

#### **3. QueueStorage** ✅ INTEGRATED
- `queue.QueueStorage` for all storage operations (txos as hashes, events as sorted sets)
- Added bulk operations: `HMSet`, `HMGet` for efficient multi-field reads/writes
- Implemented in Redis, SQLite, MongoDB, Postgres backends
- New `QueueStore` implementation: [idx/queue-store/txos.go](idx/queue-store/txos.go)

---

## Implementation Sequence

### **Phase 0: Cleanup** ✅ COMPLETED
- ✅ Removed Account Abstraction (accounts.go, owner_accounts table, routes, interface methods)
- ✅ Build passes

### **Phase 1: Config & Infrastructure** ✅ COMPLETED
- ✅ Viper-based configuration (config.yaml + env var overrides)
- ✅ Adopted Pub/Sub from overlay package
- ✅ Replaced jb package with go-junglebus + BeefStorage
- ✅ Deleted jb package

### **Phase 2: Simplify IndexContext** ✅ COMPLETED
- ✅ Removed `AncestorConfig` struct
- ✅ Removed `ancestorConfig` and `BeefStorage` fields from `IndexContext`
- ✅ Simplified `NewIndexContext()` signature (removed beefStorage, ancestorConfig params)
- ✅ Simplified `ParseSpends()` to use `SourceTransaction` directly
- ✅ Updated all callers (ingest.go, ingest/ingest.go, server/routes/tx/ctrl.go, cmd/*)
- ✅ Build passes

### **Phase 3: Integrate QueueStorage** ✅ COMPLETED

**Goal**: Replace custom TxoStore implementations with QueueStorage-based implementation.

#### 3.1 Revert u:/s: Prefix Changes ✅ COMPLETED
- ✅ Removed `u:` and `s:` prefix logic from SaveTxos (Redis, SQLite)
- ✅ Removed `ParseOwnerMember()` helper from keys.go
- ✅ Added `OwnerSpentKey()` helper for `osp:` events
- ✅ Stubbed SaveSpends() - reimplemented in QueueStore
- ✅ Updated owner routes to filter by spend status via LoadTxos (temporary)

#### 3.2 Add Bulk Hash Operations to QueueStorage ✅ COMPLETED
- ✅ Added `HMSet(ctx, key, fields map[string]string)` to interface
- ✅ Added `HMGet(ctx, key, fields ...string)` to interface
- ✅ Implemented in Redis ([overlay/queue/redis.go](../overlay/queue/redis.go))
- ✅ Implemented in SQLite ([overlay/queue/sqlite.go](../overlay/queue/sqlite.go))
- ✅ Implemented in MongoDB ([overlay/queue/mongo.go](../overlay/queue/mongo.go))
- ✅ Implemented in Postgres ([overlay/queue/postgres.go](../overlay/queue/postgres.go))

#### 3.3 Create QueueStore TxoStore Implementation ✅ COMPLETED
- ✅ Created [idx/store.go](idx/store.go) - QueueStore with all storage methods
- ✅ Created [idx/search.go](idx/search.go) - SearchCfg, Log types, search methods
- ✅ Uses QueueStorage for all operations (no direct database access)
- ✅ Simplified event keys (no `tag:` or `evt:` prefixes)

#### 3.4 Wire Up QueueStore ✅ COMPLETED
- ✅ Added QueueStorage to config via `config.QueueStorage`
- ✅ Added PubSub field to QueueStore struct
- ✅ Created `config.Store` as unified `*idx.QueueStore` instance
- ✅ Updated all routes to use `ingestCtx.Store` for lookups
- ✅ Moved `Event` type and `EventKey()` to [idx/index-data.go](idx/index-data.go)
- ✅ Changed all mod files to use `idx.Event` instead of `evt.Event`
- ✅ Changed all routes to use `idx.EventKey()` instead of `evt.EventKey()`
- ✅ Updated [ingest/ingest.go](ingest/ingest.go) to use `config.PubSub.Publish()`

#### 3.5 Remove Old Store Implementations ✅ COMPLETED
- ✅ Removed `idx/redis-store/` directory
- ✅ Removed `idx/sqlite-store/` directory
- ✅ Removed `idx/pg-store/` directory
- ✅ Removed `idx/queue-store/` directory (consolidated into idx/store.go)
- ✅ Removed `evt/` package entirely (Event moved to idx package)

**Event Key Strategy** (simplified):
```
# TXO lookup by txid
txid:{txid}                    - All outpoints for a transaction

# Owner events
own:{address}                  - TXO owned by address
osp:{address}                  - TXO spent by address

# Tag events (no prefix)
{tag}                          - TXO has this tag (e.g., "ord", "bsv21")
{tag}:{id}:{value}             - Protocol event (e.g., "ord:origin:abc123_0")
```

**Data Model**:
```
# TXO stored as hash
txo:{outpoint} -> {
  height, idx, satoshis,       # Core fields
  owners,                      # JSON array of owner addresses
  events,                      # JSON array of event keys (for rollback cleanup)
  spend,                       # Spending txid (empty if unspent)
  {tag}: {tag-specific JSON}   # Indexer data (e.g., "ord": {"origin": ...})
}
```

### **Phase 4: New Stream APIs** ⬅️ NEXT
Implement v2 event stream endpoints using QueueStore:
- `/api/v2/events/{event}` - `Search()` with event key
- `/api/v2/owner/{address}` - Query `own:` and `osp:` events
- `/api/v2/owner/{address}/utxos` - Query `own:` excluding spent
- SSE via overlay's `SSEManager`

### **Phase 5: Testing & Migration**
- Unit tests for QueueStore operations
- Integration tests with different QueueStorage backends
- Performance benchmarks vs v5
- Documentation (v6 architecture guide, fresh re-index instructions)

---

## Key Benefits

1. **Unified Storage**: Leverage overlay's battle-tested QueueStorage abstraction
2. **Performance**: Eliminate N database queries per transaction (where N = input count)
3. **Throughput**: Database becomes write-mostly, reducing contention
4. **Scalability**: Stream architecture enables horizontal scaling
5. **Flexibility**: Multiple backend support (Redis, SQLite, MongoDB, Postgres)
6. **Consistency**: Source of truth is blockchain (via BEEF), database is derived view
7. **Simplicity**: Single storage interface for all operations

---

## Files Modified (Phase 3)

### overlay Package (bulk operations added)
- ✅ `queue/queue.go` - Added `HMSet`, `HMGet` to interface
- ✅ `queue/redis.go` - Implemented bulk hash operations
- ✅ `queue/sqlite.go` - Implemented bulk hash operations
- ✅ `queue/mongo.go` - Implemented bulk hash operations
- ✅ `queue/postgres.go` - Implemented bulk hash operations

### New QueueStore Implementation
- ✅ `idx/store.go` - QueueStore struct with PubSub, all storage methods
- ✅ `idx/search.go` - SearchCfg, Log types, ComparisonType

### Event/IndexData Consolidation
- ✅ `idx/index-data.go` - Added `Event` type and `EventKey()` function
- ✅ `idx/keys.go` - `OwnerKey()`, `OwnerSpentKey()`, `BalanceKey()`, `QueueKey()`, `LogKey()`

### Config Updates
- ✅ `config/config.go` - Initialize PubSub before QueueStore, pass to `NewQueueStore()`

### Mod Files (evt.Event → idx.Event)
- ✅ `mod/shrug/shrug.go`
- ✅ `mod/onesat/origin.go`
- ✅ `mod/onesat/inscription.go`
- ✅ `mod/onesat/ordlock.go`
- ✅ `mod/onesat/bsv20.go`
- ✅ `mod/onesat/bsv21.go`
- ✅ `mod/cosign/cosign.go`
- ✅ `mod/p2pkh/p2pkh.go`
- ✅ `mod/lock/lock.go`

### Route Files (evt.EventKey → idx.EventKey)
- ✅ `server/routes/tag/ctrl.go` - Uses tag directly (no TagKey needed)
- ✅ `server/routes/origins/ctrl.go` - Uses `idx.EventKey()`
- ✅ `server/routes/evt/ctrl.go` - Uses `idx.EventKey()`

### Ingest Updates
- ✅ `ingest/ingest.go` - Changed `evt.Publish()` to `config.PubSub.Publish()`

### Removed Directories/Files
- ✅ `evt/` - Package deleted (Event moved to idx)
- ✅ `idx/redis-store/` - Removed
- ✅ `idx/sqlite-store/` - Removed
- ✅ `idx/pg-store/` - Removed
- ✅ `idx/queue-store/` - Consolidated into idx/store.go

---

## Event Key Reference

| Event Pattern | Description | Example |
|--------------|-------------|---------|
| `txid:{txid}` | All outpoints for a txid | `txid:abc123...` |
| `own:{address}` | TXO owned by address | `own:1PKH...` |
| `osp:{address}` | TXO spent by address | `osp:1PKH...` |
| `{tag}` | TXO has this tag | `ord`, `bsv21` |
| `{tag}:{id}:{value}` | Protocol event | `ord:origin:abc123_0` |

---

## Query Patterns with QueueStore

### Load TXO by outpoint
```go
txo, err := store.LoadTxo(ctx, "abc123_0", []string{"ord"}, true)
```

### Load TXOs by txid
```go
txos, err := store.LoadTxosByTxid(ctx, "abc123...", []string{"ord"}, true)
```

### Search by event key
```go
logs, err := store.Search(ctx, &SearchCfg{
    Keys:  []string{"own:1PKH..."},
    From:  &fromScore,
    Limit: 100,
})
```

### Get outpoints, then load with spend status
```go
logs, _ := store.Search(ctx, &SearchCfg{Keys: []string{"own:addr"}})
txos, _ := store.LoadTxos(ctx, outpoints, tags, true)
// Filter by txo.Spend == "" for unspent
```

---

## Resolved Decisions

- **Storage abstraction**: Use overlay's `QueueStorage` for all operations
- **Overlay package**: `github.com/b-open-io/overlay`
- **TxoStore implementation**: `QueueStore` in `idx/queue-store/txos.go`
- **Event key strategy**: No `tag:` or `evt:` prefixes - use tag directly
- **Bulk operations**: Added `HMSet`, `HMGet` to QueueStorage interface
