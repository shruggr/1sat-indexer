package server

import (
	"context"
	"log"
	"os"
	"path/filepath"

	bsv21routes "github.com/b-open-io/bsv21-overlay/routes"
	"github.com/b-open-io/overlay/queue"
	"github.com/b-open-io/overlay/routes"
	arcaderoutes "github.com/bsv-blockchain/arcade/routes/fiber"
	ctroutes "github.com/bsv-blockchain/go-chaintracks/routes/fiber"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/compress"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/shruggr/1sat-indexer/v5/config"
	"github.com/shruggr/1sat-indexer/v5/idx"
	"github.com/shruggr/1sat-indexer/v5/server/routes/evt"
	"github.com/shruggr/1sat-indexer/v5/server/routes/own"
	"github.com/shruggr/1sat-indexer/v5/server/routes/tag"
	"github.com/shruggr/1sat-indexer/v5/server/routes/tx"
	"github.com/shruggr/1sat-indexer/v5/server/routes/txos"
	ordfsapi "github.com/shruggr/go-ordfs-server/api"

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

func Initialize(ingestCtx *idx.IngestCtx, services *config.Services) *fiber.App {
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

	// Register chaintracks routes under /block/
	if services != nil && services.Chaintracks != nil {
		ctRoutes := ctroutes.NewRoutes(services.Chaintracks)
		ctRoutes.Register(app.Group("/block"))
		log.Println("Registered /block/* routes")
	}

	// Register overlay tx routes under /tx/
	if services != nil && services.BeefStorage != nil {
		routes.RegisterTxRoutes(app.Group("/tx"), &routes.TxRoutesConfig{
			BeefStorage: services.BeefStorage,
		})
		log.Println("Registered /tx/* routes")
	}

	// Register arcade routes under /arc/
	if services != nil && services.ArcadeServices != nil {
		arcadeRoutes := arcaderoutes.NewRoutes(arcaderoutes.Config{
			Service:        services.ArcadeServices.ArcadeService,
			Store:          services.ArcadeServices.Store,
			EventPublisher: services.ArcadeServices.EventPublisher,
			Arcade:         services.ArcadeServices.Arcade,
			Logger:         services.Logger,
		})
		arcadeRoutes.Register(app.Group("/arc"))
		log.Println("Registered /arc/* routes")
	}

	// Register 1sat indexer routes (no version prefix)
	evt.RegisterRoutes(app.Group("/evt"), ingestCtx)
	own.RegisterRoutes(app.Group("/owner"), ingestCtx, services.JungleBus)
	tag.RegisterRoutes(app.Group("/tag"), ingestCtx)
	txos.RegisterRoutes(app.Group("/txo"), ingestCtx)

	// Register tx controller routes for indexer-specific tx operations
	txCtrl := tx.NewTxController(
		ingestCtx,
		services.BeefStorage,
		services.PubSub,
		services.JungleBus,
	)
	txCtrl.RegisterRoutes(app.Group("/idx"))

	// Standalone callback route for ARC webhooks
	app.Post("/callback", txCtrl.TxCallback)

	// Register SSE routes
	routes.RegisterSSERoutes(app.Group("/sse"), &routes.SSERoutesConfig{
		SSEManager: services.SSEManager,
		Catchup: func(ctx context.Context, keys []string, fromScore float64) ([]queue.ScoredMember, error) {
			byteKeys := make([][]byte, len(keys))
			for i, k := range keys {
				byteKeys[i] = []byte(k)
			}
			return ingestCtx.Store.Search(ctx, &queue.SearchCfg{Keys: byteKeys, From: &fromScore}, true)
		},
		Context: context.Background(),
	})

	// Register ORDFS routes
	if services != nil && services.ORDFSServices != nil {
		// Content routes at root for proper content serving
		if err := ordfsapi.RegisterContentRoutes(app, services.ORDFSServices); err != nil {
			log.Printf("Failed to register ORDFS content routes: %v", err)
		} else {
			log.Println("Registered /content/* routes at root")
		}

		// API routes under /ordfs/
		if err := ordfsapi.RegisterAPIRoutes(app.Group("/ordfs"), services.ORDFSServices); err != nil {
			log.Printf("Failed to register ORDFS API routes: %v", err)
		} else {
			log.Println("Registered /ordfs/* API routes")
		}
	}

	// Register BSV21 routes
	if services != nil && services.BSV21Storage != nil {
		bsv21routes.RegisterBSV21Routes(app.Group("/bsv21"), &bsv21routes.BSV21RoutesConfig{
			Storage:      services.BSV21Storage,
			ChainTracker: services.Chaintracks,
			ActiveTopics: services.BSV21ActiveTopics,
			BSV21Lookup:  services.BSV21Lookup,
		})
		log.Println("Registered /bsv21/* routes")
	}

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

	return app
}
