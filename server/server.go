package server

import (
	"context"
	"log"
	"os"
	"path/filepath"

	"github.com/b-open-io/overlay/queue"
	"github.com/b-open-io/overlay/routes"
	"github.com/bsv-blockchain/go-sdk/transaction/broadcaster"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/compress"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/redis/go-redis/v9"
	"github.com/shruggr/1sat-indexer/v5/broadcast"
	"github.com/shruggr/1sat-indexer/v5/config"
	"github.com/shruggr/1sat-indexer/v5/idx"
	"github.com/shruggr/1sat-indexer/v5/server/routes/blocks"
	"github.com/shruggr/1sat-indexer/v5/server/routes/evt"
	"github.com/shruggr/1sat-indexer/v5/server/routes/origins"
	"github.com/shruggr/1sat-indexer/v5/server/routes/own"
	"github.com/shruggr/1sat-indexer/v5/server/routes/spend"
	"github.com/shruggr/1sat-indexer/v5/server/routes/tag"
	"github.com/shruggr/1sat-indexer/v5/server/routes/tx"
	"github.com/shruggr/1sat-indexer/v5/server/routes/txos"

	_ "github.com/shruggr/1sat-indexer/v5/docs"
)

// @title 1Sat Indexer API
// @version 5.0
// @description BSV blockchain indexer API for transaction outputs, blocks, and related data
// @termsOfService http://swagger.io/terms/

// @contact.name API Support
// @contact.url https://github.com/shruggr/1sat-indexer

// @license.name MIT
// @license.url https://opensource.org/licenses/MIT

// @BasePath /

func Initialize(ingestCtx *idx.IngestCtx, arcBroadcaster *broadcaster.Arc) *fiber.App {
	app := fiber.New(fiber.Config{
		BodyLimit: 100 * 1024 * 1024, // 100MB
	})
	// app.Use(recover.New())
	app.Use(logger.New())
	app.Use(compress.New())
	app.Use(cors.New(cors.Config{AllowOrigins: "*"}))

	// @Summary Health check
	// @Description Simple health check endpoint
	// @Tags health
	// @Success 200 {string} string "yo"
	// @Router /yo [get]
	app.Get("/yo", func(c *fiber.Ctx) error {
		return c.SendString("yo")
	})

	v5 := app.Group("/v5")

	blocks.RegisterRoutes(v5.Group("/blocks"))
	evt.RegisterRoutes(v5.Group("/evt"), ingestCtx)
	origins.RegisterRoutes(v5.Group("/origins"), ingestCtx)
	own.RegisterRoutes(v5.Group("/own"), ingestCtx)
	tag.RegisterRoutes(v5.Group("/tag"), ingestCtx)
	tx.RegisterRoutes(v5.Group("/tx"), ingestCtx, arcBroadcaster)
	txos.RegisterRoutes(v5.Group("/txo"), ingestCtx)
	spend.RegisterRoutes(v5.Group("/spends"), ingestCtx)

	// Register SSE routes using overlay's implementation
	routes.RegisterSSERoutes(v5, &routes.SSERoutesConfig{
		SSEManager: config.SSEManager,
		Catchup: func(ctx context.Context, keys []string, fromScore float64) ([]queue.ScoredMember, error) {
			return ingestCtx.Store.Search(ctx, &idx.SearchCfg{Keys: keys, From: &fromScore})
		},
		Context: config.Ctx,
	})

	// Get current working directory
	cwd, _ := os.Getwd()
	log.Printf("Server working directory: %s", cwd)

	// Try to find docs folder in multiple locations
	possiblePaths := []string{
		"./docs",
		"../../docs",
		filepath.Join(cwd, "docs"),
	}

	var docsPath string
	for _, p := range possiblePaths {
		absPath, _ := filepath.Abs(p)
		swaggerPath := filepath.Join(absPath, "swagger.json")
		if _, err := os.Stat(swaggerPath); err == nil {
			docsPath = absPath
			log.Printf("✓ Found swagger.json at: %s", swaggerPath)
			break
		}
	}

	if docsPath == "" {
		log.Printf("✗ swagger.json not found in any of these locations:")
		for _, p := range possiblePaths {
			absPath, _ := filepath.Abs(p)
			log.Printf("  - %s", filepath.Join(absPath, "swagger.json"))
		}
		docsPath = "./docs" // fallback
	}

	app.Static("/api-spec", docsPath)

	app.Get("/docs", func(c *fiber.Ctx) error {
		html := `<!doctype html>
<html>
<head>
    <title>1Sat Indexer API</title>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
</head>
<body>
    <script id="api-reference" data-url="/api-spec/swagger.json"></script>
    <script src="https://cdn.jsdelivr.net/npm/@scalar/api-reference"></script>
</body>
</html>`
		c.Set("Content-Type", "text/html")
		return c.SendString(html)
	})

	// Broadcast status listener for ARC callbacks
	go func() {
		if opts, err := redis.ParseURL(os.Getenv("REDISEVT")); err != nil {
			log.Printf("Error parsing REDISEVT URL for broadcast listener: %v", err)
			return
		} else {
			statusRedis := redis.NewClient(opts)
			listener := broadcast.InitListener(statusRedis)
			ctx := context.Background()
			listener.Start(ctx)
		}
	}()

	return app
}
