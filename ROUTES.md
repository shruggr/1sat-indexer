# 1sat-indexer Route Inventory

This document catalogs all HTTP routes across the unified server components, categorized by data domain and integration status.

---

## Route Status Legend

| Symbol | Meaning |
|--------|---------|
| ✅ | Currently wired into 1sat-indexer |
| ⚠️ | Available but NOT wired |
| 🔄 | Duplicate/overlapping functionality |
| 🗑️ | Candidate for removal |

---

## Routes by Data Domain

### 1. Block & Chain Data

Routes that interact with blockchain headers, tips, and chain state.

| Status | Method | Path | Source | Handler | Description |
|--------|--------|------|--------|---------|-------------|
| ✅ | GET | `/v5/blocks/tip` | 1sat-indexer | `BlocksController.GetChaintip` | Current chain tip |
| ✅ | GET | `/v5/blocks/height/{height}` | 1sat-indexer | `BlocksController.GetBlockByHeight` | Block by height |
| ✅ | GET | `/v5/blocks/hash/{hash}` | 1sat-indexer | `BlocksController.GetBlockByHash` | Block by hash |
| ✅ | GET | `/v5/blocks/list/{from}` | 1sat-indexer | `BlocksController.ListBlocks` | List blocks (up to 10k) |
| ⚠️ | GET | `/block/tip` | overlay | `common.go` | Chain tip (overlay format) |
| ⚠️ | GET | `/block/:height` | overlay | `common.go` | Block header by height |
| ✅ | GET | `/chaintracks/v2/network` | go-chaintracks | `HandleGetNetwork` | Network name |
| ✅ | GET | `/chaintracks/v2/height` | go-chaintracks | `HandleGetHeight` | Current height |
| ✅ | GET | `/chaintracks/v2/tip` | go-chaintracks | `HandleGetTip` | Chain tip (full header) |
| ✅ | GET | `/chaintracks/v2/tip/stream` | go-chaintracks | `HandleTipStream` | SSE tip updates |
| ✅ | GET | `/chaintracks/v2/header/height/:height` | go-chaintracks | `HandleGetHeaderByHeight` | Header by height |
| ✅ | GET | `/chaintracks/v2/header/hash/:hash` | go-chaintracks | `HandleGetHeaderByHash` | Header by hash |
| ✅ | GET | `/chaintracks/v2/headers` | go-chaintracks | `HandleGetHeaders` | Bulk headers (binary) |
| ✅ | GET | `/ordfs/v1/bsv/block/latest` | go-ordfs-server | `v1BlockHandler.GetLatest` | Latest block |
| ✅ | GET | `/ordfs/v1/bsv/block/height/:height` | go-ordfs-server | `v1BlockHandler.GetByHeight` | Block by height |
| ✅ | GET | `/ordfs/v1/bsv/block/hash/:hash` | go-ordfs-server | `v1BlockHandler.GetByHash` | Block by hash |
| ✅ | GET | `/ordfs/v2/block/tip` | go-ordfs-server | `v2BlockHandler.GetTip` | Merkle root bytes |
| ✅ | HEAD | `/ordfs/v2/block/tip` | go-ordfs-server | `v2BlockHandler.GetTip` | Tip status check |
| ✅ | GET | `/ordfs/v2/chain/height` | go-ordfs-server | `v2BlockHandler.GetChainHeight` | Plain text height |
| ✅ | GET | `/ordfs/v2/block/:hashOrHeight` | go-ordfs-server | `v2BlockHandler.GetBlockHeader` | Block by hash or height |

**Analysis:** Significant overlap in block/chain endpoints across 4 packages. Consider consolidating to chaintracks as the authoritative source.

---

### 2. Transaction Data

Routes for fetching, parsing, and querying transactions.

| Status | Method | Path | Source | Handler | Description |
|--------|--------|------|--------|---------|-------------|
| ✅ | GET | `/v5/tx/{txid}` | 1sat-indexer | `TxController.GetTxWithProof` | Tx with merkle proof |
| ✅ | GET | `/v5/tx/{txid}/raw` | 1sat-indexer | `TxController.GetRawTx` | Raw tx (bin/hex/json) |
| ✅ | GET | `/v5/tx/{txid}/proof` | 1sat-indexer | `TxController.GetProof` | Merkle proof only |
| ✅ | GET | `/v5/tx/{txid}/beef` | 1sat-indexer | `TxController.GetTxBEEF` | BEEF format |
| ✅ | GET | `/v5/tx/{txid}/txos` | 1sat-indexer | `TxController.TxosByTxid` | All TXOs from tx |
| ✅ | GET | `/v5/tx/{txid}/parse` | 1sat-indexer | `TxController.ParseTx` | Parse & return indexed data |
| ✅ | POST | `/v5/tx/parse` | 1sat-indexer | `TxController.ParseTx` | Parse posted tx bytes |
| ⚠️ | GET | `/beef/:topic/:txid` | overlay | `common.go` | BEEF by topic & txid |
| ✅ | GET | `/ordfs/v1/bsv/tx/:txid` | go-ordfs-server | `v1TxHandler.GetRawTx` | Raw tx bytes |
| ✅ | GET | `/ordfs/v2/tx/:txid` | go-ordfs-server | `v2TxHandler.GetRawTx` | Raw tx (binary) |
| ✅ | GET | `/ordfs/v2/tx/:txid/proof` | go-ordfs-server | `v2TxHandler.GetMerkleProof` | Merkle proof |
| ✅ | GET | `/ordfs/v2/tx/:txid/beef` | go-ordfs-server | `v2TxHandler.GetBeef` | BEEF format |
| ✅ | GET | `/ordfs/v2/tx/:txid/:outputIndex` | go-ordfs-server | `v2TxHandler.GetOutput` | Specific output bytes |

**Analysis:** Transaction fetching duplicated between 1sat-indexer and ORDFS. ORDFS routes likely use different data sources (remote fetching vs local index).

---

### 3. Transaction Broadcasting

Routes for submitting transactions to the network.

| Status | Method | Path | Source | Handler | Description |
|--------|--------|------|--------|---------|-------------|
| ✅ | POST | `/v5/tx` | 1sat-indexer | `TxController.BroadcastTx` | Broadcast tx |
| ✅ | POST | `/v5/tx/{txid}/ingest` | 1sat-indexer | `TxController.IngestTx` | Force ingest by txid |
| ✅ | POST | `/v5/tx/callback` | 1sat-indexer | `TxController.TxCallback` | ARC callback receiver |
| ⚠️ | POST | `/api/v1/submit` | overlay | `submit.go` | BEEF submit with topics |
| ✅ | POST | `/arcade/tx` | arcade | `handlePostTx` | Submit single tx |
| ✅ | POST | `/arcade/txs` | arcade | `handlePostTxs` | Submit multiple txs |
| ✅ | GET | `/arcade/tx/:txid` | arcade | `handleGetTx` | Get tx status |
| ✅ | GET | `/arcade/policy` | arcade | `handleGetPolicy` | Get policy limits |
| ✅ | GET | `/arcade/health` | arcade | `handleGetHealth` | Health check |
| ✅ | GET | `/arcade/events/:callbackToken` | arcade | `handleTxSSE` | SSE status stream |

**Analysis:** Broadcasting split between 1sat-indexer (simple), overlay (topic-aware BEEF), and arcade (full ARC implementation). Arcade is the most complete.

---

### 4. Transaction Outputs (TXOs)

Routes for querying indexed transaction outputs.

| Status | Method | Path | Source | Handler | Description |
|--------|--------|------|--------|---------|-------------|
| ✅ | GET | `/v5/txo/{outpoint}` | 1sat-indexer | `GetTxo` | Single TXO lookup |
| ✅ | POST | `/v5/txo` | 1sat-indexer | `GetTxos` | Batch TXO lookup |

**Analysis:** Core indexer functionality. Keep as-is.

---

### 5. Spend Tracking

Routes for tracking output spends.

| Status | Method | Path | Source | Handler | Description |
|--------|--------|------|--------|---------|-------------|
| ✅ | GET | `/v5/spends/{outpoint}` | 1sat-indexer | `GetSpend` | Spend info for outpoint |
| ✅ | POST | `/v5/spends` | 1sat-indexer | `GetSpends` | Batch spend lookup |

**Analysis:** Core indexer functionality. Keep as-is.

---

### 6. Origin & History (1Sat Ordinals)

Routes for 1Sat origin tracking and history.

| Status | Method | Path | Source | Handler | Description |
|--------|--------|------|--------|---------|-------------|
| ✅ | GET | `/v5/origins/history/{outpoint}` | 1sat-indexer | `OriginHistory` | Origin history |
| ✅ | POST | `/v5/origins/history` | 1sat-indexer | `OriginsHistory` | Batch history |
| ✅ | GET | `/v5/origins/ancestors/{outpoint}` | 1sat-indexer | `OriginAncestors` | Origin ancestors |
| ✅ | POST | `/v5/origins/ancestors` | 1sat-indexer | `OriginsAncestors` | Batch ancestors |

**Analysis:** Core 1Sat functionality. Keep as-is.

---

### 7. Owner/Account Queries

Routes for querying by owner address or pubkey.

| Status | Method | Path | Source | Handler | Description |
|--------|--------|------|--------|---------|-------------|
| ✅ | GET | `/v5/own/{owner}/txos` | 1sat-indexer | `OwnerTxos` | All TXOs for owner |
| ✅ | GET | `/v5/own/{owner}/utxos` | 1sat-indexer | `OwnerUtxos` | Unspent TXOs |
| ✅ | GET | `/v5/own/{owner}/balance` | 1sat-indexer | `OwnerBalance` | Satoshi balance |
| ✅ | GET | `/v5/own/{owner}/sync` | 1sat-indexer | `OwnerSync` | Paginated wallet sync |

**Analysis:** Core indexer functionality. Keep as-is.

---

### 8. Event/Topic Queries (Overlay)

Routes for topic-scoped event queries.

| Status | Method | Path | Source | Handler | Description |
|--------|--------|------|--------|---------|-------------|
| ✅ | GET | `/v5/evt/{tag}/{id}/{value}` | 1sat-indexer | `TxosByEvent` | TXOs by event |
| ✅ | GET | `/v5/tag/{tag}` | 1sat-indexer | `TxosByTag` | TXOs by tag |
| ⚠️ | GET | `/events/:topic/:event/history` | overlay | `common.go` | Event history |
| ⚠️ | POST | `/events/:topic/history` | overlay | `common.go` | Batch event history |
| ⚠️ | GET | `/events/:topic/:event/unspent` | overlay | `common.go` | Unspent by event |
| ⚠️ | POST | `/events/:topic/unspent` | overlay | `common.go` | Batch unspent |

**Analysis:** 1sat-indexer has simplified event routes. Overlay routes are more flexible with topic scoping. Consider whether topic-aware routes are needed.

---

### 9. Server-Sent Events (SSE)

Real-time streaming routes.

| Status | Method | Path | Source | Handler | Description |
|--------|--------|------|--------|---------|-------------|
| ✅ | GET | `/v5/sse/subscribe` | overlay | `sse.go` | SSE subscription |
| ✅ | POST | `/v5/sse/unsubscribe` | overlay | `sse.go` | SSE unsubscribe |
| ✅ | GET | `/chaintracks/v2/tip/stream` | go-chaintracks | `HandleTipStream` | Chain tip SSE |
| ✅ | GET | `/arcade/events/:callbackToken` | arcade | `handleTxSSE` | Tx status SSE |

**Analysis:** Multiple SSE endpoints for different purposes. Keep separate.

---

### 10. Content & ORDFS

Routes for serving inscription content and ORDFS filesystem.

| Status | Method | Path | Source | Handler | Description |
|--------|--------|------|--------|---------|-------------|
| ✅ | GET | `/content/*` | go-ordfs-server | `ContentHandler.HandleAll` | Content by pointer |
| ✅ | GET | `/ordfs/v2/metadata/*` | go-ordfs-server | `v2MetadataHandler.GetMetadata` | Inscription metadata |
| ✅ | GET | `/ordfs/v2/stream/:outpoint` | go-ordfs-server | `streamHandler.HandleStream` | Chunked streaming |
| ✅ | GET | `/preview/:b64HtmlData` | go-ordfs-server | `frontendHandler.RenderPreview` | Preview HTML |
| ✅ | POST | `/preview` | go-ordfs-server | `frontendHandler.RenderPreviewPost` | POST preview |
| ✅ | GET | `/*` (DNS catch-all) | go-ordfs-server | `DNSHandler` | Domain-based routing |

**Analysis:** ORDFS is specialized content serving. Keep as separate domain.

---

### 11. Utility Routes

Health checks, documentation, and miscellaneous.

| Status | Method | Path | Source | Handler | Description |
|--------|--------|------|--------|---------|-------------|
| ✅ | GET | `/yo` | 1sat-indexer | anonymous | Health check |
| ✅ | GET | `/docs` | 1sat-indexer | Scalar | API documentation UI |
| ✅ | GET | `/api-spec/*` | 1sat-indexer | Static | OpenAPI spec files |
| ✅ | GET | `/health` | go-ordfs-server | anonymous | Health check |
| ✅ | GET | `/arcade/health` | arcade | `handleGetHealth` | Arcade health |
| ✅ | GET | `/ordfs/v1/docs/*` | go-ordfs-server | Swagger | V1 API docs |
| ✅ | GET | `/ordfs/v2/docs/*` | go-ordfs-server | Swagger | V2 API docs |

---

## Unwired Routes Summary

Routes defined but NOT currently registered in 1sat-indexer:

### From overlay package (`routes/common.go`)
```
GET  /events/:topic/:event/history   - Event history lookup
POST /events/:topic/history          - Batch event history
GET  /events/:topic/:event/unspent   - Unspent by event
POST /events/:topic/unspent          - Batch unspent
GET  /block/tip                      - Chain tip
GET  /block/:height                  - Block by height
GET  /beef/:topic/:txid              - BEEF by topic
```

### From overlay package (`routes/submit.go`)
```
POST /api/v1/submit                  - BEEF submit with topics + peer broadcast
```

---

## Consolidation Opportunities

### 1. Block/Chain Endpoints (HIGH PRIORITY)
**Current state:** 4 packages implement block endpoints
**Recommendation:** Use chaintracks as single source of truth
- Remove: `/v5/blocks/*` from 1sat-indexer
- Remove: `/ordfs/v*/block/*` from go-ordfs-server
- Keep: `/chaintracks/v2/*` as canonical

### 2. Transaction Fetching (MEDIUM PRIORITY)
**Current state:** 1sat-indexer and ORDFS both serve raw tx/proof/beef
**Recommendation:** Determine primary use case
- If fetching from local index: 1sat-indexer
- If fetching from remote/network: ORDFS
- Consider: Proxy pattern or single implementation

### 3. Broadcasting (MEDIUM PRIORITY)
**Current state:** 3 broadcast implementations
**Recommendation:** Use arcade as primary broadcaster
- `/arcade/tx` - Full ARC implementation with status tracking
- Remove: `/v5/tx` POST from 1sat-indexer (or proxy to arcade)
- Wire: `/api/v1/submit` for topic-aware overlay submissions

### 4. Event Queries (LOW PRIORITY)
**Current state:** Simple 1sat routes vs topic-scoped overlay routes
**Recommendation:** Evaluate if topic scoping is needed
- If yes: Wire overlay's `/events/*` routes
- If no: Keep current `/v5/evt/*` and `/v5/tag/*`

---

## Proposed Clean Route Structure

```
/                           # Root
├── /health                 # Unified health check
├── /docs                   # API documentation
│
├── /v1/                    # Legacy/compatibility (if needed)
│
├── /chain/                 # Block & chain data (chaintracks)
│   ├── GET  /tip           # Current tip
│   ├── GET  /tip/stream    # SSE tip updates
│   ├── GET  /height        # Current height
│   ├── GET  /header/:id    # By height or hash
│   └── GET  /headers       # Bulk binary headers
│
├── /tx/                    # Transaction operations
│   ├── GET  /:txid         # Tx with proof
│   ├── GET  /:txid/raw     # Raw bytes
│   ├── GET  /:txid/proof   # Merkle proof
│   ├── GET  /:txid/beef    # BEEF format
│   ├── POST /              # Broadcast (arcade)
│   ├── POST /submit        # Topic-aware BEEF submit
│   └── GET  /status/:txid  # Broadcast status
│
├── /txo/                   # Transaction outputs
│   ├── GET  /:outpoint     # Single TXO
│   └── POST /              # Batch TXOs
│
├── /spend/                 # Spend tracking
│   ├── GET  /:outpoint     # Single spend
│   └── POST /              # Batch spends
│
├── /origin/                # 1Sat origins
│   ├── GET  /:outpoint/history    # History
│   ├── GET  /:outpoint/ancestors  # Ancestors
│   └── POST /history              # Batch history
│
├── /owner/                 # Owner queries
│   ├── GET  /:owner/txos   # All TXOs
│   ├── GET  /:owner/utxos  # Unspent
│   ├── GET  /:owner/balance # Balance
│   └── GET  /:owner/sync   # Wallet sync
│
├── /event/                 # Event queries
│   ├── GET  /:topic/:event # By event
│   └── GET  /:tag          # By tag
│
├── /sse/                   # Real-time streams
│   ├── GET  /subscribe     # General subscription
│   └── GET  /tx/:token     # Tx status stream
│
├── /content/               # ORDFS content
│   └── GET  /*             # Content by pointer
│
└── /ordfs/                 # ORDFS API
    ├── GET  /metadata/*    # Inscription metadata
    └── GET  /stream/:out   # Chunked streaming
```

---

## Data Flow Dependencies

```
┌─────────────────────────────────────────────────────────────────┐
│                        External Data                             │
├─────────────────────────────────────────────────────────────────┤
│  JungleBus    │  Teranode P2P  │  ARC API   │  Remote Nodes     │
└───────┬───────┴───────┬────────┴─────┬──────┴────────┬──────────┘
        │               │              │               │
        ▼               ▼              ▼               ▼
┌───────────────┐ ┌───────────┐ ┌──────────┐ ┌─────────────────┐
│  subscribe    │ │ chaintracks│ │  arcade  │ │ ordfs (remote)  │
│  (indexing)   │ │ (headers) │ │(broadcast)│ │ (content fetch) │
└───────┬───────┘ └─────┬─────┘ └────┬─────┘ └────────┬────────┘
        │               │            │                │
        ▼               ▼            ▼                ▼
┌─────────────────────────────────────────────────────────────────┐
│                      Storage Layer                               │
├─────────────────────────────────────────────────────────────────┤
│  PostgreSQL/SQLite  │  Redis (queue/cache)  │  In-memory        │
└─────────────────────┴───────────────────────┴───────────────────┘
        │
        ▼
┌─────────────────────────────────────────────────────────────────┐
│                      API Layer (this server)                     │
├─────────────────────────────────────────────────────────────────┤
│  /chain/*  │  /tx/*  │  /txo/*  │  /origin/*  │  /content/*     │
└─────────────────────────────────────────────────────────────────┘
```

---

## Next Steps

1. **Decide on canonical block/chain source** - Likely chaintracks
2. **Decide on canonical broadcaster** - Likely arcade
3. **Wire unwired overlay routes** if topic-scoping needed
4. **Remove redundant 1sat-indexer routes** that duplicate external packages
5. **Update API documentation** to reflect consolidated structure
6. **Consider versioning strategy** - Clean break vs. gradual migration
