const env = {
    POSTGRES_FULL: "postgres://postgres:postgres@127.0.0.1:5432/ordinals?sslmode=disable",
    POSTGRES_READ: "postgres://postgres:postgres@127.0.0.1:5432/ordinals?sslmode=disable",
    JUNGLEBUS: "https://junglebus.gorillapool.io",

    REDISCACHE: "127.0.0.1:6380",
    REDISDB: "127.0.0.1:6666",
    ARC: "https://arc.gorillapool.io",
    TAAL_TOKEN: "mainnet_3c4ab60e633fa8524272900e5603ca7c",
    INDEXER: "http://localhost:8082",
}

module.exports = {
    apps: [
        {
            name: "block",
            script: "./block-sync",
            env,
        }, {
            name: "ord",
            script: "./subscribe",
            args: "-t=3c0bc5dcbc4e7b4919d7b51d6ebf2ba42afae314ea57500cc8d4a71e5e3d1957 -s=783968 -v=1",
            env,
        }, {
            name: "ingest",
            script: "./ingest",
            env,
        }
    ]
}

