# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Repository Overview

1sat-indexer is a Bitcoin SV (BSV) transaction indexer that tracks ordinals and 1Sat origins. It provides a comprehensive indexing system for BSV transactions with support for multiple protocols and real-time transaction processing.

### Key Concepts

**Ordinal Indexing** (Not yet functioning): A system for tracking unique serial numbers (ordinals) assigned to individual satoshis by walking back the blockchain.

**1Sat Origin Indexing**: BSV's unique ability to support single-satoshi outputs enables efficient tracking of satoshi origins. An "origin" is defined as the first outpoint where a satoshi exists alone in a one-satoshi output. This allows for efficient indexing without a full ordinal indexer.

## Common Development Commands

### Building
```bash
./build.sh           # Build all executables (full, ingest, owners, server, subscribe)
go run cmd/full/full.go      # Run full service directly
go run cmd/ingest/ingest.go  # Run ingest service directly
go run cmd/server/server.go  # Run API server directly
```

### Testing
```bash
go test ./...                    # Run all tests
go test ./pkg/name -run TestName # Run specific test
go test -v ./...                 # Verbose test output
```

### Development
```bash
go mod tidy          # Clean up dependencies
```

## Architecture Overview

### Service Components

The indexer follows a multi-service architecture:

1. **subscribe** - JungleBus subscriber that receives blockchain transactions and queues them
2. **ingest** - Processes queued transactions, runs indexers, and handles Arc callbacks
3. **full** - Combined service that runs owner sync + ingest + API server
4. **server** - HTTP API server for querying indexed data
5. **owner-sync** - Syncs transactions by owner address

### Data Flow

```
JungleBus → Subscribe → Redis Queue → Ingest → Storage (PostgreSQL/SQLite/Redis)
                                         ↓
                              Protocol Parsers (Indexers)
                                         ↓
                                   API Server → SSE/REST
```

### Core Architecture Patterns

**Indexer Pattern**: All protocol parsers implement the `Indexer` interface with methods:
- `Tag()` - Returns indexer identifier
- `Parse(idxCtx *IndexContext, vout uint32)` - Parses transaction outputs
- `PreSave(idxCtx *IndexContext)` - Pre-save processing
- `FromBytes(data []byte)` - Deserialize indexed data

**Transaction Scoring**: Transactions are scored using `HeightScore(height, idx)`:
- Block transactions: `height * 1000000000 + blockIndex`
- Mempool transactions: `UnixNano` timestamp
- Broadcast attempts (unconfirmed): Negative `UnixNano` timestamp
- Immutable threshold: Transactions >10 blocks deep

**Storage Abstraction**: `TxoStore` interface supports multiple backends:
- PostgreSQL (`idx/pg-store/`)
- SQLite (`idx/sqlite-store/`)
- Redis (`idx/redis-store/`)

### Ingestion Pipeline

The ingest service manages three main responsibilities:

1. **Queue Processing** - Processes transactions from Redis queue with configurable concurrency
2. **Arc Callback Listener** - Handles transaction status updates from Arc broadcaster
3. **Transaction Auditing** - Periodically verifies transaction states:
   - Negative scores: Unconfirmed broadcasts (>2min old) - check Arc status
   - Mined transactions: Verify MerklePath and check for immutability (>10 blocks)
   - Old mempool: Transactions >3hr without proof (configurable rollback)

### Command-Line Flags

**subscribe**:
- `-tag` - Subscription tag (required)
- `-t` - JungleBus topic/subscription ID (required)
- `-q` - Queue name (default: IngestTag)
- `-s` - Start from block height
- `-m` - Index mempool transactions
- `-b` - Index block transactions (default: true)
- `-r` - Enable reorg rewind
- `-v` - Verbose logging level

**ingest**:
- `-tag` - Log tag (default: IngestTag)
- `-q` - Queue tag (default: IngestTag)
- `-c` - Concurrency level (default: 1)
- `-v` - Verbose logging level
- `-r` - Enable rollback for old mempool transactions
- `-l` - Load ancestors
- `-p` - Parse ancestors (default: true)
- `-s` - Save ancestors (default: true)

**full**:
- `-p` - Port to listen on
- `-tag` - Log tag (default: IngestTag)
- `-q` - Queue tag (default: IngestTag)
- `-c` - Concurrency level (default: 1)
- `-v` - Verbose logging level

## Environment Variables

Required environment variables (typically in `.env`):
- `POSTGRES_FULL` - PostgreSQL connection string
- `JUNGLEBUS` - JungleBus endpoint (e.g., https://junglebus.gorillapool.io)
- `ARC` - Arc broadcaster endpoint (e.g., https://arc.gorillapool.io)
- `REDIS` - Redis host:port
- `REDISEVT` - Redis URL for event publishing
- `TAAL_TOKEN` - TAAL API token (if using TAAL for Arc)

## Protocol Modules

Protocol parsers are located in `mod/`:
- `bitcom/` - Bitcoin computer protocols (B, MAP, SIGMA)
- `cosign/` - Co-signing protocol
- `lock/` - Locking scripts
- `onesat/` - 1Sat Ordinals (origins, inscriptions, BSV20, BSV21, ordlock)
- `opns/` - OPNS protocol
- `p2pkh/` - Pay-to-pubkey-hash
- `shrug/` - Shrug protocol

Each protocol module can register itself with the global `config.Indexers` slice.

## Key Packages

- `blk/` - Block header management and chain tip tracking
- `broadcast/` - Transaction broadcasting and Arc status monitoring
- `cmd/` - Executable entry points
- `config/` - Global configuration and indexer registration
- `evt/` - Event publishing to Redis
- `idx/` - Core indexing logic, storage interfaces, and TXO management
- `ingest/` - Transaction ingestion pipeline with queue processing and auditing
- `jb/` - JungleBus client for loading transactions and subscriptions
- `lib/` - Utility types (Outpoint, PKHash, Network, etc.)
- `migration/` - Database migrations
- `mod/` - Protocol-specific indexers
- `server/` - HTTP API server with REST and SSE endpoints
- `sub/` - JungleBus subscription management

## API Routes (v5)

- `/v5/acct/*` - Account-based queries
- `/v5/blocks/*` - Block information
- `/v5/evt/*` - Event queries
- `/v5/origins/*` - 1Sat origin lookups
- `/v5/own/*` - Owner-based queries
- `/v5/spends/*` - Spend tracking
- `/v5/sse` - Server-sent events for real-time updates
- `/v5/tag/*` - Tag-based searches
- `/v5/tx/*` - Transaction operations and broadcast
- `/v5/txo/*` - Transaction output queries

## Transaction Lifecycle

1. **Subscription**: JungleBus publishes transaction → Subscribe service receives it
2. **Queueing**: Transaction logged to Redis queue with score (height-based or timestamp)
3. **Ingestion**: Ingest worker picks from queue → Creates IndexContext → Runs all registered indexers
4. **Parsing**: Each indexer parses relevant outputs → Generates IndexData
5. **Storage**: TXOs saved to store with indexed data → Spends recorded
6. **Auditing**: Periodic verification of transaction states and MerklePaths
7. **Events**: SSE notifications published for subscribed clients

## Arc Integration

The indexer integrates with Arc (Authoritative Response Component) for transaction broadcasting and status tracking:

- Broadcast attempts tracked with negative scores (timestamp)
- Arc callbacks update transaction status via Redis pub/sub
- Three Arc callback scenarios:
  1. **Mined** - MerklePath provided → Re-ingest with proof
  2. **Rejected** - Transaction invalid → Rollback from index
  3. **Pending** - Status check → Update or remove from queue

## Development Notes

- Use `go run` for development instead of building to avoid artifact cleanup
- Service startup order for full indexing: subscribe → ingest → server
- The indexer uses `go-sdk` with a specific commit hash via replace directive in go.mod
- Redis is used for queuing, caching, and event pub/sub
- PostgreSQL/SQLite for persistent indexed data storage
- SSE (Server-Sent Events) provides real-time updates to clients
