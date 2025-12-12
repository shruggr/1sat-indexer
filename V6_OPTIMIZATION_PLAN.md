# 1sat-indexer v6 Optimization Plan

## Overview

Stream-based architecture using overlay's `QueueStorage` for all storage. TXOs stored as hashes, events stored as sorted sets.

---

## Event Keys

| Pattern | Description | Example |
|---------|-------------|---------|
| `txid:{txid}` | All outpoints for a txid | `txid:abc123...` |
| `own:{address}` | TXO owned by address | `own:1PKH...` |
| `own:{address}:spnd` | TXO spent by owner | `own:1PKH...:spnd` |
| `{tag}` | TXO has this tag | `ord`, `bsv21` |
| `{tag}:{id}:{value}` | Protocol event | `ord:origin:abc123_0` |

**Score format**: `height * 1e6 + idx` for chronological ordering.

---

## Data Model

```
# TXO stored as hash
txo:{outpoint} -> {
  height, idx, satoshis,
  owners,                    # JSON array of owner addresses
  events,                    # JSON array of event keys
  spend,                     # Spending txid (empty if unspent)
  {tag}: {tag-specific JSON} # Indexer data
}

# Events stored as sorted sets
own:1PKH...      -> [{member: "abc123_0", score: 800000000000.0}]
own:1PKH...:spnd -> [{member: "abc123_0", score: 800001500000.0}]
```

---

## Wallet Sync API

### `GET /v5/own/:owner/sync`

Paginated endpoint for wallet synchronization. Returns a merged stream of outputs and spends ordered by score.

**Parameters**:
- `from` - Starting score for pagination (default: 0)
- `limit` - Max results (default: 100)

**Response**:
```json
{
  "outputs": [
    {"outpoint": "abc_0", "score": 800000000000, "spendTxid": "def456..."},
    {"outpoint": "abc_0", "score": 800001500000, "spendTxid": "def456..."}
  ],
  "nextScore": 800001500000,
  "done": false
}
```

**Behavior**:
- Merges `own:{address}` (outputs) and `own:{address}:spnd` (spends) into a single stream
- Same outpoint can appear twice at different scores (once when created, once when spent)
- `spendTxid` is populated for all outputs that have been spent, regardless of which event triggered the entry
- Search deduplicates by (member, score) - same member at same score is skipped

**Client usage**:
- Start with `from=0`, page through until `done=true`
- Use `nextScore` as `from` for subsequent requests
- Track outpoints client-side; update spend status when `spendTxid` appears
- Only persist scores ≥5 blocks behind chain tip (reorg safety)

---

## Implementation Status

### Phase 0-4: ✅ COMPLETED

- Removed Account Abstraction
- Viper-based configuration
- Adopted Pub/Sub and QueueStorage from overlay
- Simplified IndexContext to use SourceTransaction directly
- Created QueueStore implementation in `idx/store.go`
- Integrated SSE from overlay

### Phase 5: Wallet Sync ✅ COMPLETED

- Added `GET /v5/own/:owner/sync` endpoint
- Single merged query of `own:{address}` and `own:{address}:spnd` streams
- Search allows same member at different scores (dedupes only same member + same score)
- Returns unified `outputs` array with `spendTxid` populated for spent outputs

### Phase 6: Testing & Documentation

- Unit tests for QueueStore operations
- Integration tests with different backends
- Performance benchmarks (low priority)

---

## Key Files

- `idx/store.go` - QueueStore implementation
- `idx/search.go` - Search with FilterSpent support
- `idx/keys.go` - Key helpers (`OwnerKey`, `OwnerSpentKey`)
- `server/routes/own/ctrl.go` - Owner routes including sync endpoint
