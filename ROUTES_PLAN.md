# Routes Plan - API Consolidation Review

This document catalogs all routes across the merged services to identify overlaps, redundancies, and consolidation opportunities.

## Current Route Structure in 1sat-indexer

The main server (`server/server.go`) mounts routes at these prefixes:
- `/v5/*` - 1sat-indexer native routes
- `/chaintracks/*` - go-chaintracks routes
- `/arcade/*` - arcade routes
- `/` - Root level (health, docs)

---

## Category 1: Health & Documentation Routes

| Route | Method | Source | Description |
|-------|--------|--------|-------------|
| `/yo` | GET | 1sat-indexer | Simple health check |
| `/docs` | GET | 1sat-indexer | Swagger UI |
| `/api-spec/*` | Static | 1sat-indexer | Swagger JSON |
| `/arcade/health` | GET | arcade | Arcade health check |
| `/arcade/policy` | GET | arcade | Transaction policy info |
| `/chaintracks/network` | GET | chaintracks | Network name (mainnet/testnet) |

**go-ordfs-server standalone:**
- `/health` - Health check
- `/v1/docs/*` - V1 Swagger
- `/v2/docs/*` - V2 Swagger

---

## Category 2: Block/Chain Header Routes

| Route | Method | Source | Description |
|-------|--------|--------|-------------|
| `/v5/blocks/tip` | GET | 1sat-indexer | Get chain tip |
| `/v5/blocks/height/:height` | GET | 1sat-indexer | Get block by height |
| `/v5/blocks/hash/:hash` | GET | 1sat-indexer | Get block by hash |
| `/v5/blocks/list/:from` | GET | 1sat-indexer | List blocks from height |
| `/chaintracks/network` | GET | chaintracks | Get network name |
| `/chaintracks/height` | GET | chaintracks | Get chain height only |
| `/chaintracks/tip` | GET | chaintracks | Get chain tip header |
| `/chaintracks/tip/stream` | GET | chaintracks | SSE stream of tip updates |
| `/chaintracks/header/height/:height` | GET | chaintracks | Get header by height |
| `/chaintracks/header/hash/:hash` | GET | chaintracks | Get header by hash |
| `/chaintracks/headers` | GET | chaintracks | Get multiple headers (binary) |

**go-ordfs-server standalone:**
- `/v1/bsv/block/latest` - Get latest block
- `/v1/bsv/block/height/:height` - Get block by height
- `/v1/bsv/block/hash/:hash` - Get block by hash
- `/v2/block/tip` - Get chain tip
- `/v2/chain/height` - Get chain height
- `/v2/block/:hashOrHeight` - Get block header

**Overlap Analysis:**
- `/v5/blocks/*` and `/chaintracks/*` provide similar block header functionality
- ordfs has its own block routes that could use chaintracks internally

---

## Category 3: Transaction Routes

### 3a. Transaction Broadcasting

| Route | Method | Source | Description |
|-------|--------|--------|-------------|
| `/v5/tx` | POST | 1sat-indexer | Broadcast tx (also ingests) |
| `/v5/tx/callback` | POST | 1sat-indexer | ARC callback handler |
| `/arcade/tx` | POST | arcade | Broadcast single tx (ARC-compatible) |
| `/arcade/txs` | POST | arcade | Broadcast multiple txs |

**Overlap Analysis:**
- Both 1sat-indexer and arcade can broadcast transactions
- 1sat-indexer's broadcast also ingests; arcade's is pure ARC-style broadcast
- Different callback mechanisms: 1sat uses `/v5/tx/callback`, arcade uses event publisher

### 3b. Transaction Retrieval

| Route | Method | Source | Description |
|-------|--------|--------|-------------|
| `/v5/tx/:txid` | GET | 1sat-indexer | Get tx with proof (varint-length prefixed) |
| `/v5/tx/:txid/raw` | GET | 1sat-indexer | Get raw tx (bin/hex/json) |
| `/v5/tx/:txid/proof` | GET | 1sat-indexer | Get merkle proof |
| `/v5/tx/:txid/beef` | GET | 1sat-indexer | Get tx in BEEF format |
| `/v5/tx/:txid/txos` | GET | 1sat-indexer | Get TXOs for tx |
| `/v5/tx/:txid/parse` | GET | 1sat-indexer | Parse tx and return index data |
| `/v5/tx/parse` | POST | 1sat-indexer | Parse provided tx |
| `/v5/tx/:txid/ingest` | POST | 1sat-indexer | Force ingest tx |
| `/arcade/tx/:txid` | GET | arcade | Get tx status (submission tracking) |

**go-ordfs-server standalone:**
- `/v1/bsv/tx/:txid` - Get raw tx
- `/v2/tx/:txid` - Get raw tx
- `/v2/tx/:txid/proof` - Get merkle proof
- `/v2/tx/:txid/beef` - Get tx as BEEF
- `/v2/tx/:txid/:outputIndex` - Get specific output

**Overlap Analysis:**
- 1sat-indexer `/v5/tx/:txid` and ordfs `/v2/tx/:txid` serve different purposes (with-proof vs raw)
- BEEF and proof endpoints exist in both
- arcade's `/tx/:txid` is for broadcast status, not tx data

---

## Category 4: TXO (Transaction Output) Routes

| Route | Method | Source | Description |
|-------|--------|--------|-------------|
| `/v5/txo/:outpoint` | GET | 1sat-indexer | Get single TXO |
| `/v5/txo` | POST | 1sat-indexer | Get multiple TXOs |
| `/v5/spends/:outpoint` | GET | 1sat-indexer | Get spend info for outpoint |
| `/v5/spends` | POST | 1sat-indexer | Get spend info for multiple outpoints |

**go-ordfs-server standalone:**
- `/v2/tx/:txid/:outputIndex` - Get specific output data

---

## Category 5: Origin Routes (1sat-specific)

| Route | Method | Source | Description |
|-------|--------|--------|-------------|
| `/v5/origins/ancestors` | POST | 1sat-indexer | Get ancestors for multiple origins |
| `/v5/origins/ancestors/:outpoint` | GET | 1sat-indexer | Get ancestors for single origin |
| `/v5/origins/history` | POST | 1sat-indexer | Get history for multiple origins |
| `/v5/origins/history/:outpoint` | GET | 1sat-indexer | Get history for single origin |

---

## Category 6: Owner Routes (1sat-specific)

| Route | Method | Source | Description |
|-------|--------|--------|-------------|
| `/v5/own/:owner/txos` | GET | 1sat-indexer | Get all TXOs for owner |
| `/v5/own/:owner/utxos` | GET | 1sat-indexer | Get unspent TXOs for owner |
| `/v5/own/:owner/balance` | GET | 1sat-indexer | Get balance for owner |
| `/v5/own/:owner/sync` | GET | 1sat-indexer | Sync TXOs for owner |

---

## Category 7: Tag/Event Routes (1sat-specific)

| Route | Method | Source | Description |
|-------|--------|--------|-------------|
| `/v5/tag/:tag` | GET | 1sat-indexer | Get TXOs by tag |
| `/v5/evt/:tag/:id/:value` | GET | 1sat-indexer | Get TXOs by event |

---

## Category 8: SSE (Server-Sent Events) Routes

| Route | Method | Source | Description |
|-------|--------|--------|-------------|
| `/v5/subscribe/:events` | GET | 1sat-indexer (overlay) | Subscribe to events |
| `/chaintracks/tip/stream` | GET | chaintracks | Stream tip updates |
| `/arcade/events/:callbackToken` | GET | arcade | Stream tx status updates |

**Overlap Analysis:**
- Three different SSE mechanisms for different purposes
- Overlay's SSE is for general event streaming
- Chaintracks SSE is for block tip updates only
- Arcade SSE is for transaction status tracking

---

## Category 9: Content/DNS Routes (ordfs-specific)

**go-ordfs-server standalone:**
- `/v2/metadata/*` - Get metadata
- `/v2/stream/:outpoint` - Stream content
- `/preview/:b64HtmlData` - Render preview
- `/preview` (POST) - Render preview
- `/content/*` - Content handler
- `/*` - DNS/wildcard handler

---

## Summary of Overlaps & Recommendations

### Clear Overlaps to Address:

1. **Block Headers**: `/v5/blocks/*` vs `/chaintracks/*`
   - Consider deprecating one in favor of the other
   - chaintracks has more features (streaming, binary headers)

2. **Transaction Broadcasting**: `/v5/tx` POST vs `/arcade/tx` POST
   - Different purposes: 1sat ingests after broadcast, arcade is pure ARC
   - May want to keep both but clarify use cases

3. **Transaction Retrieval**: Multiple ways to get tx data
   - 1sat-indexer, ordfs both have raw/beef/proof endpoints
   - Need to clarify which is the "source of truth"

### Non-overlapping (unique to each):

- **1sat-indexer only**: Origins, Owners, Tags, Events, TXO queries
- **arcade only**: Transaction status tracking, batch broadcast
- **chaintracks only**: Height-only endpoint, tip streaming, binary headers
- **ordfs only**: Content serving, DNS handling, metadata, streaming

---

## Questions for Discussion

1. Should `/v5/blocks/*` be deprecated in favor of `/chaintracks/*`?
2. Should transaction broadcast go through arcade exclusively, or keep both paths?
3. Should ordfs be integrated as routes in 1sat-indexer or remain standalone?
4. How should SSE routes be consolidated or kept separate?
