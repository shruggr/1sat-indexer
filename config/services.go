package config

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/b-open-io/bsv21-overlay/lookups"
	"github.com/b-open-io/go-junglebus"
	"github.com/b-open-io/overlay/beef"
	"github.com/b-open-io/overlay/pubsub"
	"github.com/b-open-io/overlay/queue"
	"github.com/b-open-io/overlay/storage"
	"github.com/b-open-io/overlay/topics"
	arcadeconfig "github.com/bsv-blockchain/arcade/config"
	"github.com/bsv-blockchain/go-chaintracks/chaintracks"
	p2p "github.com/bsv-blockchain/go-teranode-p2p-client"
	ordfsconfig "github.com/shruggr/go-ordfs-server/config"

	"github.com/shruggr/1sat-indexer/v5/idx"
	"github.com/shruggr/1sat-indexer/v5/lib"
)

// Services holds all initialized services for the indexer.
type Services struct {
	// Store is the TXO storage backend
	Store *storage.OutputStore

	// QueueStorage is the underlying queue storage backend
	QueueStorage queue.QueueStorage

	// PubSub is the event publishing backend
	PubSub pubsub.PubSub

	// SSEManager manages SSE client subscriptions
	SSEManager *pubsub.SSEManager

	// Network is the BSV network (mainnet/testnet)
	Network lib.Network

	// P2PClient is the P2P network client
	P2PClient *p2p.Client

	// Chaintracks is the chain tracker for SPV validation
	Chaintracks chaintracks.Chaintracks

	// JungleBus is the JungleBus client
	JungleBus *junglebus.Client

	// BeefStorage is the BEEF storage chain
	BeefStorage *beef.Storage

	// Indexers is the list of active indexers
	Indexers []idx.Indexer

	// ArcadeServices holds arcade-related services (when using embedded arcade)
	ArcadeServices *arcadeconfig.Services

	// ORDFSServices holds ordfs-specific services (loader, cache)
	ORDFSServices *ordfsconfig.Services

	// BSV21 services
	BSV21Storage      *storage.EventDataStorage
	BSV21Lookup       *lookups.Bsv21EventsLookup
	BSV21ActiveTopics *topics.ActiveTopics

	// Logger for the services
	Logger *slog.Logger

	// Config holds the configuration used to create services
	Config *Config
}

// Initialize creates all services from configuration.
// The logger parameter is optional (nil = use default logger).
func (c *Config) Initialize(ctx context.Context, logger *slog.Logger) (*Services, error) {
	if logger == nil {
		logger = slog.Default()
	}

	services := &Services{
		Config: c,
		Logger: logger,
	}

	var err error

	// Initialize PubSub first (needed by QueueStore and SSEManager)
	services.PubSub, err = pubsub.CreatePubSub(c.PubSub.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize pubsub: %w", err)
	}
	logger.Info("Initialized pubsub", slog.String("url", c.PubSub.URL))

	// Initialize SSEManager for server-sent events
	services.SSEManager = pubsub.NewSSEManager(ctx, services.PubSub)
	logger.Info("Initialized SSE manager")

	// Initialize QueueStorage backend based on store.type
	var storeURL string
	switch c.Store.Type {
	case "badger", "":
		// Badger is the default
		storePath := c.Store.Path
		if storePath == "" {
			storePath = "~/.1sat/indexer"
		}
		storeURL = "badger://" + storePath
		services.QueueStorage, err = queue.NewBadgerQueueStorage(storeURL, logger)
	default:
		err = fmt.Errorf("unsupported store type: %s (only 'badger' is currently supported)", c.Store.Type)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to initialize queue storage: %w", err)
	}

	// Create OutputStore with PubSub
	services.Store = storage.NewOutputStore(services.QueueStorage, services.PubSub, "")
	logger.Info("Initialized queue storage", slog.String("url", storeURL))

	// Initialize JungleBus
	if c.Network.JungleBus != "" {
		services.JungleBus, err = junglebus.New(junglebus.WithHTTP(c.Network.JungleBus))
		if err != nil {
			return nil, fmt.Errorf("failed to initialize junglebus: %w", err)
		}
		logger.Info("Initialized JungleBus", slog.String("url", c.Network.JungleBus))
	}

	// Initialize P2P client
	logger.Info("Initializing P2P client")
	c.P2P.Network = c.Network.Type
	services.P2PClient, err = c.P2P.Initialize(ctx, "1sat-indexer")
	if err != nil {
		return nil, fmt.Errorf("failed to initialize p2p client: %w", err)
	}
	logger.Info("Initialized P2P client")

	// Initialize Chaintracks (as its own service, sharing P2P client)
	logger.Info("Initializing Chaintracks")
	services.Chaintracks, err = c.Chaintracks.Initialize(ctx, "1sat-indexer", services.P2PClient)
	if err != nil {
		_ = services.P2PClient.Close()
		return nil, fmt.Errorf("failed to initialize chaintracks: %w", err)
	}
	logger.Info("Initialized Chaintracks")

	// Initialize Arcade (receives the existing Chaintracks and P2P client)
	logger.Info("Initializing Arcade services")
	services.ArcadeServices, err = c.Arcade.Initialize(ctx, logger, services.Chaintracks, services.P2PClient)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize arcade: %w", err)
	}

	// Initialize ORDFS (shares Chaintracks, Redis from its own config)
	// Only initialize if Redis URL is configured
	if c.Ordfs.Redis.URL != "" {
		logger.Info("Initializing ORDFS services")
		services.ORDFSServices, err = c.Ordfs.Initialize(ctx, services.Chaintracks, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize ordfs: %w", err)
		}
		logger.Info("Initialized ORDFS services")
	}

	// Initialize BEEF storage
	services.BeefStorage, err = beef.NewStorage(c.Beef.URL, services.Chaintracks)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize beef storage: %w", err)
	}
	logger.Info("Initialized BEEF storage", slog.String("url", c.Beef.URL))

	// Initialize BSV21 services if enabled
	if c.BSV21.Enabled {
		logger.Info("Initializing BSV21 services")

		// Create BSV21 EventDataStorage - shares QueueStorage, PubSub, and BeefStorage with main indexer
		services.BSV21Storage = storage.CreateEventDataStorage(
			services.QueueStorage,
			services.PubSub,
			services.BeefStorage,
		)
		logger.Info("Initialized BSV21 event storage (shared dependencies)")

		// Create BSV21 lookup service
		services.BSV21Lookup, err = lookups.NewBsv21EventsLookup(services.BSV21Storage)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize BSV21 lookup: %w", err)
		}
		logger.Info("Initialized BSV21 lookup service")

		// Create ActiveTopics cache (refresh starts when routes are registered)
		services.BSV21ActiveTopics = topics.NewActiveTopics()
		logger.Info("Initialized BSV21 ActiveTopics cache")
	}

	// Set network
	switch c.Network.Type {
	case "main":
		services.Network = lib.Mainnet
	case "test":
		services.Network = lib.Testnet
	default:
		services.Network = lib.Mainnet
	}

	// Initialize indexers
	services.Indexers = CreateIndexers(c.Indexers)
	logger.Info("Initialized indexers", slog.Any("indexers", c.Indexers))

	return services, nil
}

// Close gracefully shuts down all services.
func (s *Services) Close() error {
	// Close ORDFS services if we created them
	if s.ORDFSServices != nil {
		if err := s.ORDFSServices.Close(); err != nil {
			return err
		}
	}

	// Close BSV21 storage if we created it
	if s.BSV21Storage != nil {
		if err := s.BSV21Storage.Close(); err != nil {
			return err
		}
	}

	// Close arcade services if we created them
	// Note: arcade Services doesn't have a Close method yet, but we should add cleanup here
	return nil
}
