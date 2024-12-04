package main

import (
	"context"

	"github.com/shruggr/1sat-indexer/v5/audit"
)

func main() {
	audit.StartTxAudit(context.Background())
}
