go build -o audit.run cmd/audit/audit.go
go build -o ingest.run cmd/ingest/ingest.go
go build -o origin.run cmd/origin/origin.go
go build -o owners.run cmd/owner-sync/owner-sync.go
go build -o server.run cmd/server/server.go
go build -o subscribe.run cmd/subscribe/subscribe.go
go build -o bsv21.run cmd/bsv21/bsv21.go
