# 1sat-indexer

## Ordinal Theory
Ordinal Theory defines a means of walking back the blockchain to determin a unique serial number (ordinal) assigned to a single satoshi. Details of ordinals can be found here: [https://docs.ordinals.com/]

Indexing the ordinal number of an individual satoshi can be very expensive in terms of processing, RAM, and storage. 1Sat indexing takes a different approach where, if you can determine that multiple transactions are spending the SAME Ordinal, it doesn't matter WHICH Ordinal Number that is.


## 1Sat Origin Indexing
The BSV blockchain is unique among blockchains which support ordinals, in that BSV supports single satoshi outputs. This allows us to take some short-cuts in indexing until a full ordinal indexer can be built efficiently. 

Since ordinals are a unique serial number for each satoshi, an `origin` can be defined as the first outpoint where a satoshi exists alone, in a one satoshi output. Each subsequent spend of that satoshi will be crawled back only to the first ancestor where the origin has already been identified, or until it's ancestor is an output which contains more than one satoshi.

If a satoshi is subsequently packaged up in an output of more than one satoshi, the origin is no longer carried forward. If the satoshi is later spent into another one satoshi output, a new origin will be created. Both of these origins would be the same ordinal, and when the ordinal indexer is complete, both those origins will be identified as both being the same ordinal.

### Prerequesites
Postgres >= v10
Redis (Currently assumes running on localhost with the default port)
Bitcoin SV node
Go >= v1.20

### Environment Variables
- POSTGRES_FULL=`<postgres connection string>`
- BITCOIN_HOST
- BITCOIN_PORT
- BITCOIN_USER
- BITCOIN_PASS

## Run DB migrations
```
cd migrations
go run .
```
## Run Inscriptions Indexer against a Bitcoin SV node
```
cd node
go build
./node
```
