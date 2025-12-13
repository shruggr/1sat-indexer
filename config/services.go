package config

import (
	"context"
	"fmt"
	"log/slog"
	"path"

	"github.com/b-open-io/go-junglebus"
	"github.com/b-open-io/overlay/beef"
	"github.com/b-open-io/overlay/pubsub"
	"github.com/b-open-io/overlay/queue"
	arcadeconfig "github.com/bsv-blockchain/arcade/config"
	"github.com/bsv-blockchain/go-chaintracks/chaintracks"
	"github.com/bsv-blockchain/go-sdk/transaction/broadcaster"
	p2p "github.com/bsv-blockchain/go-teranode-p2p-client"

	"github.com/shruggr/1sat-indexer/v5/idx"
	"github.com/shruggr/1sat-indexer/v5/lib"
)

// Services holds all initialized services for the indexer.
type Services struct {
	// Store is the TXO storage backend
	Store *idx.QueueStore

	// QueueStorage is the underlying queue storage backend
	QueueStorage queue.QueueStorage

	// PubSub is the event publishing backend
	PubSub pubsub.PubSub

	// SSEManager manages SSE client subscriptions
	SSEManager *pubsub.SSEManager

	// Broadcaster is the transaction broadcaster
	Broadcaster *broadcaster.Arc

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

	// Initialize QueueStorage backend
	switch c.Store.Type {
	case "sqlite":
		services.QueueStorage, err = queue.NewSQLiteQueueStorage(c.Store.SQLite)
	case "redis":
		services.QueueStorage, err = queue.NewRedisQueueStorage(c.Store.Redis)
	case "postgres":
		services.QueueStorage, err = queue.NewPostgresQueueStorage(c.Store.Postgres)
	default:
		return nil, fmt.Errorf("unknown store type: %s", c.Store.Type)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to initialize queue storage: %w", err)
	}

	// Create QueueStore with PubSub
	services.Store = idx.NewQueueStore(services.QueueStorage, services.PubSub)
	logger.Info("Initialized queue storage", slog.String("type", c.Store.Type))

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
	if c.P2P.StoragePath == "" {
		c.P2P.StoragePath = "~/.1sat"
	}
	services.P2PClient, err = c.P2P.Initialize(ctx, "1sat-indexer")
	if err != nil {
		return nil, fmt.Errorf("failed to initialize p2p client: %w", err)
	}
	logger.Info("Initialized P2P client")

	// Initialize Chaintracks (as its own service, sharing P2P client)
	logger.Info("Initializing Chaintracks")
	if c.Chaintracks.StoragePath == "" {
		c.Chaintracks.StoragePath = path.Join("~/.1sat", "chaintracks")
	}
	services.Chaintracks, err = c.Chaintracks.Initialize(ctx, "1sat-indexer", services.P2PClient)
	if err != nil {
		_ = services.P2PClient.Close()
		return nil, fmt.Errorf("failed to initialize chaintracks: %w", err)
	}
	logger.Info("Initialized Chaintracks")

	// Initialize Arcade (receives the existing Chaintracks instance)
	logger.Info("Initializing Arcade services")
	services.ArcadeServices, err = c.Arcade.Initialize(ctx, logger, services.Chaintracks)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize arcade: %w", err)
	}

	// Initialize BEEF storage
	services.BeefStorage, err = beef.NewStorage(c.Beef.URL, services.Chaintracks)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize beef storage: %w", err)
	}
	logger.Info("Initialized BEEF storage", slog.String("url", c.Beef.URL))

	// Initialize ARC broadcaster
	if c.Arc.URL != "" {
		maxTimeout := 10
		var callbackURL, callbackToken *string
		if c.Arc.Callback != "" {
			callbackURL = &c.Arc.Callback
		}
		if c.Arc.Token != "" {
			callbackToken = &c.Arc.Token
		}
		services.Broadcaster = &broadcaster.Arc{
			ApiUrl:        c.Arc.URL,
			ApiKey:        c.Arc.APIKey,
			WaitFor:       broadcaster.ACCEPTED_BY_NETWORK,
			MaxTimeout:    &maxTimeout,
			CallbackUrl:   callbackURL,
			CallbackToken: callbackToken,
		}
		logger.Info("Initialized ARC broadcaster", slog.String("url", c.Arc.URL))
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
	// Close arcade services if we created them
	// Note: arcade Services doesn't have a Close method yet, but we should add cleanup here
	return nil
}
