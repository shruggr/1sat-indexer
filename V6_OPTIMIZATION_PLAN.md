# 1sat-indexer v6 Optimization Plan

## Overview
Transform 1sat-indexer from relational database-centric to stream-based architecture, eliminating database read bottlenecks and enabling efficient UTXO tracking through dual-event streams.

---

## Architectural Decisions

### 1. **Dual-Event Stream Architecture**
- **Single sorted set per index key** with event type prefixes
- **Unspent events**: `u:{outpoint}` (e.g., `u:abc123_0`)
- **Spent events**: `s:{outpoint}` (e.g., `s:abc123_0`)
- Both events logged to all relevant keys (owners, tags, event keys)
- Chronologically ordered by score: `height * 1e9 + blockIndex`

**Example sorted set**:
```
own:1PKH... -> {
  u:abc123_0: 800000000000.0,  // unspent (created) at height 800000
  s:abc123_0: 800001500000.0,  // spent at height 800001
  u:def456_1: 800001000000.0   // unspent at height 800001
}
```

### 2. **Mandatory Ancestor Re-parsing**
- Remove database lookup path in `ParseSpends()` (idx/index-context.go:95)
- Always load source transactions from JungleBus and re-parse
- Database becomes **write-mostly derived view**, not source of truth during indexing
- Source transactions are **required** for proper indexing (hard requirement)

### 3. **API Strategy**
- **Priority**: New stream-focused endpoints first
  - `/api/v2/events/{owner}` - Paginated event catchup
  - `/api/v2/events/{owner}/stream` - SSE with dual events
  - Stream APIs expose `u:` and `s:` event types with scores
- **Legacy APIs**: De-prioritized, may not function during transition
  - Search functionality will likely be completely reworked
  - Disable/stub old endpoints as needed to maintain buildability
  - Backward compatibility decisions deferred until v6 architecture is proven

### 4. **Storage Layer**
- **Primary**: SQLite (embedded, serverless)
- **Primary**: Redis (production, native pub/sub)
- **Pub/Sub**: Add abstraction layer with in-memory implementation
- **Future**: BadgerDB exploration deferred

### 5. **Migration Strategy**
- v6 requires **fresh re-index from genesis** (no migration tools)
- Clean slate ensures all data has dual-event format
- Users can run v5/v6 side-by-side during transition

---

## Integration with overlay Package

### Components to Adopt from `/Users/davidcase/Source/wallet/overlay`:

#### **1. BEEF Storage** (overlay/beef/storage.go)
- **Interface**: `BeefStorage` with `LoadBeef()`, `SaveBeef()`, `UpdateMerklePath()`
- **Purpose**: Replace JungleBus direct calls with hierarchical BEEF caching
- **Implementations available**:
  - LRU cache (overlay/beef/lru.go) - in-memory with size limits
  - Redis (overlay/beef/redis.go)
  - SQLite (overlay/beef/sqlite.go)
  - Filesystem (overlay/beef/filesystem.go)
  - JungleBus (overlay/beef/junglebus.go) - network fallback
- **Factory**: Chainable storage layers via connection strings (overlay/beef/factory.go)
  - Example: `"lru://100mb,redis://localhost:6379,junglebus://"`
  - Creates: LRU → Redis → JungleBus fallback chain

**Benefit**: Optimized ancestor transaction loading with multi-tier caching

#### **2. In-Memory Pub/Sub** (overlay/pubsub/channels.go)
- **Type**: `ChannelPubSub` - pure Go, no dependencies
- **Interface**: `PubSub` with `Publish()`, `Subscribe()`, `Unsubscribe()`
- **Features**:
  - sync.Map for topic subscriptions
  - Buffered channels (100 events)
  - Context-based cleanup
  - Handles slow consumers gracefully
- **Use case**: SQLite standalone mode without Redis

**Benefit**: Enables SSE functionality without Redis server

#### **3. Pub/Sub Abstraction** (overlay/pubsub/pubsub.go)
- **Unified Event structure**: `Topic`, `Member`, `Score`, `Source`, `Data`
- **Implementations**:
  - Redis pub/sub (overlay/pubsub/redis.go)
  - Channels (in-memory) (overlay/pubsub/channels.go)
- **Configuration**: `SubscriberConfig` for flexible backend selection

**Benefit**: Clean abstraction allows swapping Redis/in-memory based on deployment

---

## Implementation Sequence

### **Phase 1: Core Architecture Changes**
1. **Modify ParseSpends** (idx/index-context.go:88-138)
   - Remove `Store.LoadTxo()` database lookup
   - Make ancestor re-parsing mandatory
   - Integrate BEEF storage from overlay package

2. **Integrate BEEF Storage**
   - Add overlay/beef as dependency
   - Replace JungleBus direct calls with `beef.LoadTx()`
   - Configure layered caching: `LRU → Redis/SQLite → JungleBus`

3. **Dual-Event Logging**
   - Update `SaveTxos()` to log `u:{outpoint}` events (idx/redis-store/txos.go:144, idx/pg-store/txos.go)
   - Update `SaveSpends()` to log `s:{outpoint}` events (idx/redis-store/txos.go:217)
   - Log spend events to all relevant keys (owners, tags, evt keys)

### **Phase 2: Pub/Sub Abstraction**
4. **Add Pub/Sub Interface**
   - Adopt `pubsub.PubSub` interface from overlay
   - Implement wrapper for existing Redis pub/sub (evt/event.go)
   - Add ChannelPubSub for standalone mode

5. **Update Event Publishing**
   - Modify `evt.Publish()` to use new abstraction
   - Publish both creation and spend events
   - Include event type in published data

### **Phase 3: New Stream APIs**
6. **Implement v2 Stream Endpoints**
   - `/api/v2/events/{owner}` - paginated event catchup
   - `/api/v2/events/{owner}/stream` - SSE with dual events
   - Expose event types (`u:`/`s:`) and scores
   - Build UTXO sets by processing events in order

7. **Legacy API Handling** (Deferred)
   - Stub or disable incompatible v1 endpoints as needed
   - Focus on buildability, not backward compatibility
   - Search functionality redesign happens after v6 core is stable

### **Phase 4: Storage Implementation**
8. **SQLite Dual-Event Support**
   - Update schema/queries for `u:` and `s:` prefixes
   - Test UTXO queries with event filtering

9. **Redis Dual-Event Support**
   - Update sorted set operations
   - Verify score-based range queries

### **Phase 5: Testing & Migration**
10. **Testing**
    - Unit tests for dual-event logic
    - Integration tests with BEEF storage chain
    - Performance benchmarks vs v5

11. **Documentation**
    - v6 architecture guide
    - Fresh re-index instructions
    - BEEF storage configuration examples

---

## Key Benefits

1. **Performance**: Eliminate N database queries per transaction (where N = input count)
2. **Throughput**: Database becomes write-mostly, reducing contention
3. **Scalability**: Stream architecture enables horizontal scaling
4. **Flexibility**: Layered BEEF caching adapts to deployment needs
5. **Consistency**: Source of truth is blockchain (via BEEF), database is derived view
6. **Features**: Complete event lifecycle tracking enables new query patterns

---

## Files to Modify

### Core Indexing
- idx/index-context.go:88-138 - Remove DB lookups, integrate BEEF
- idx/keys.go - Document prefix conventions

### Storage Implementations
- idx/redis-store/txos.go - Dual-event logging
- idx/pg-store/txos.go - Dual-event logging (deprecate or maintain)
- idx/redis-store/search.go - UTXO filtering with events
- idx/pg-store/search.go - UTXO filtering with events

### Event System
- evt/event.go - Pub/sub abstraction, spend events

### API
- server/routes/ - New v2 stream endpoints
- server/server.go - SSE with dual events

### Configuration
- Add BEEF storage configuration
- Add pub/sub backend selection

---

## Next Steps

1. Review and approve this plan
2. Prioritize which phases to tackle first
3. Validate BEEF storage integration approach
4. Confirm any additional overlay utilities to adopt
