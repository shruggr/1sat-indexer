cd ./cmd/ord
go build .
cd ../market
go build .
cd ../market-spends
go build .
cd ../locks
go build .
cd ../opns
go build .
cd ../bsv20-analysis
go build .
cd ../bsv20-deploy
go build .
cd ../bsv20v1
go build .
cd ../bsv20v2
go build .
cd ../mempool
go build .
cd ../clean-mempool
go build 
cd ../sigil
go build .
cd ../../