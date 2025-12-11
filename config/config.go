package config

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/b-open-io/go-junglebus"
	"github.com/b-open-io/overlay/beef"
	"github.com/b-open-io/overlay/pubsub"
	"github.com/b-open-io/overlay/queue"
	"github.com/bsv-blockchain/arcade"
	"github.com/bsv-blockchain/arcade/chaintracks"
	"github.com/bsv-blockchain/go-sdk/transaction/broadcaster"
	"github.com/spf13/viper"

	"github.com/shruggr/1sat-indexer/v5/idx"
	"github.com/shruggr/1sat-indexer/v5/lib"
)

// Config holds all configuration for the indexer
type Config struct {
	// Storage
	Store struct {
		Type     string `mapstructure:"type"`     // sqlite, redis, postgres
		SQLite   string `mapstructure:"sqlite"`   // path for sqlite
		Redis    string `mapstructure:"redis"`    // redis URL
		Postgres string `mapstructure:"postgres"` // postgres URL
	} `mapstructure:"store"`

	// PubSub for event publishing
	PubSub struct {
		URL string `mapstructure:"url"` // redis://, channels://
	} `mapstructure:"pubsub"`

	// BEEF storage chain
	Beef struct {
		URL string `mapstructure:"url"` // connection string chain
	} `mapstructure:"beef"`

	// Network settings
	Network struct {
		Type       string `mapstructure:"type"`       // main, test
		JungleBus  string `mapstructure:"junglebus"`  // JungleBus URL
		Chaintracks string `mapstructure:"chaintracks"` // Chaintracks URL (empty = local arcade)
		Bootstrap  string `mapstructure:"bootstrap"`  // Bootstrap URL for local arcade
	} `mapstructure:"network"`

	// ARC broadcaster
	Arc struct {
		URL      string `mapstructure:"url"`
		APIKey   string `mapstructure:"api_key"`
		Callback string `mapstructure:"callback"`
		Token    string `mapstructure:"token"`
	} `mapstructure:"arc"`

	// Server settings
	Server struct {
		Port int `mapstructure:"port"`
	} `mapstructure:"server"`

	// Indexers to enable
	Indexers []string `mapstructure:"indexers"`
}

// Load reads configuration from file and environment variables.
// Config file locations (in order of precedence):
//   - ./config.yaml
//   - ~/.1sat/config.yaml
//   - /etc/1sat/config.yaml
//
// Environment variables override config file values with prefix "INDEXER_".
// Example: INDEXER_STORE_TYPE=redis overrides store.type
func Load() (*Config, error) {
	v := viper.New()

	// Set defaults
	v.SetDefault("store.type", "sqlite")
	v.SetDefault("store.sqlite", "~/.1sat/indexer.db")
	v.SetDefault("pubsub.url", "channels://")
	v.SetDefault("beef.url", "lru://?size=100mb,~/.1sat/beef,junglebus://")
	v.SetDefault("network.type", "main")
	v.SetDefault("network.junglebus", "https://junglebus.gorillapool.io")
	v.SetDefault("server.port", 8080)
	v.SetDefault("indexers", []string{"p2pkh", "lock", "inscription", "ordlock"})

	// Config file settings
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	v.AddConfigPath("$HOME/.1sat")
	v.AddConfigPath("/etc/1sat")

	// Environment variable settings
	v.SetEnvPrefix("INDEXER")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Also support legacy env vars without prefix for backward compatibility
	bindLegacyEnvVars(v)

	// Read config file (optional - env vars can provide everything)
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("error reading config file: %w", err)
		}
		// Config file not found is OK - use defaults and env vars
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("error unmarshaling config: %w", err)
	}

	// Expand ~ in paths
	cfg.Store.SQLite = expandPath(cfg.Store.SQLite)
	cfg.Beef.URL = expandBeefPath(cfg.Beef.URL)

	return &cfg, nil
}

// bindLegacyEnvVars binds old-style env vars for backward compatibility
func bindLegacyEnvVars(v *viper.Viper) {
	// Storage
	v.BindEnv("store.sqlite", "SQLITE")
	v.BindEnv("store.redis", "REDISTXO")
	v.BindEnv("store.postgres", "POSTGRES_FULL")

	// PubSub (legacy REDISEVT becomes pubsub.url if it's a redis URL)
	v.BindEnv("pubsub.url", "PUBSUB_URL", "REDISEVT")

	// BEEF
	v.BindEnv("beef.url", "BEEF_URL")

	// Network
	v.BindEnv("network.type", "NETWORK")
	v.BindEnv("network.junglebus", "JUNGLEBUS")
	v.BindEnv("network.chaintracks", "CHAINTRACKS_URL")
	v.BindEnv("network.bootstrap", "BOOTSTRAP_URL")

	// ARC
	v.BindEnv("arc.url", "ARC_URL")
	v.BindEnv("arc.api_key", "ARC_API_KEY")
	v.BindEnv("arc.callback", "ARC_CALLBACK")
	v.BindEnv("arc.token", "ARC_TOKEN")

	// Server
	v.BindEnv("server.port", "PORT")
}

// expandPath expands ~ to home directory
func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}

// expandBeefPath expands ~ in BEEF connection string (which may have multiple paths)
func expandBeefPath(url string) string {
	parts := strings.Split(url, ",")
	for i, part := range parts {
		parts[i] = expandPath(part)
	}
	return strings.Join(parts, ",")
}

// Initialize creates all shared resources from the configuration.
// This should be called once at application startup.
func (c *Config) Initialize(ctx context.Context) error {
	var err error

	// Initialize PubSub first (needed by QueueStore and SSEManager)
	PubSub, err = pubsub.CreatePubSub(c.PubSub.URL)
	if err != nil {
		return fmt.Errorf("failed to initialize pubsub: %w", err)
	}
	log.Printf("Initialized pubsub: %s", c.PubSub.URL)

	// Initialize SSEManager for server-sent events
	SSEManager = pubsub.NewSSEManager(ctx, PubSub)
	log.Println("Initialized SSE manager")

	// Initialize QueueStorage backend
	switch c.Store.Type {
	case "sqlite":
		QueueStorage, err = queue.NewSQLiteQueueStorage(c.Store.SQLite)
	case "redis":
		QueueStorage, err = queue.NewRedisQueueStorage(c.Store.Redis)
	case "postgres":
		QueueStorage, err = queue.NewPostgresQueueStorage(c.Store.Postgres)
	default:
		return fmt.Errorf("unknown store type: %s", c.Store.Type)
	}
	if err != nil {
		return fmt.Errorf("failed to initialize queue storage: %w", err)
	}

	// Create QueueStore with PubSub
	Store = idx.NewQueueStore(QueueStorage, PubSub)
	log.Printf("Initialized %s queue storage", c.Store.Type)

	// Initialize JungleBus
	if c.Network.JungleBus != "" {
		JungleBus, err = junglebus.New(junglebus.WithHTTP(c.Network.JungleBus))
		if err != nil {
			return fmt.Errorf("failed to initialize junglebus: %w", err)
		}
		log.Printf("Initialized JungleBus: %s", c.Network.JungleBus)
	}

	// Initialize Chaintracks
	if c.Network.Chaintracks != "" {
		log.Printf("Using remote chaintracks at %s", c.Network.Chaintracks)
		Chaintracks = chaintracks.NewClient(c.Network.Chaintracks)
		Chaintracks.SubscribeTip(ctx)
	} else {
		log.Println("Running arcade locally")
		arcadeInstance, err := arcade.NewArcade(ctx, arcade.Config{
			Network:      c.Network.Type,
			BootstrapURL: c.Network.Bootstrap,
		})
		if err != nil {
			return fmt.Errorf("failed to initialize local arcade: %w", err)
		}
		if err := arcadeInstance.Start(ctx); err != nil {
			return fmt.Errorf("failed to start local arcade: %w", err)
		}
		Chaintracks = arcadeInstance
	}

	// Initialize BEEF storage
	BeefStorage, err = beef.NewStorage(c.Beef.URL, Chaintracks)
	if err != nil {
		return fmt.Errorf("failed to initialize beef storage: %w", err)
	}
	log.Printf("Initialized BEEF storage: %s", c.Beef.URL)

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
		Broadcaster = &broadcaster.Arc{
			ApiUrl:        c.Arc.URL,
			ApiKey:        c.Arc.APIKey,
			WaitFor:       broadcaster.ACCEPTED_BY_NETWORK,
			MaxTimeout:    &maxTimeout,
			CallbackUrl:   callbackURL,
			CallbackToken: callbackToken,
		}
		log.Printf("Initialized ARC broadcaster: %s", c.Arc.URL)
	}

	// Set network
	switch c.Network.Type {
	case "main":
		Network = lib.Mainnet
	case "test":
		Network = lib.Testnet
	default:
		Network = lib.Mainnet
	}

	// Initialize indexers
	Indexers = CreateIndexers(c.Indexers)
	log.Printf("Initialized indexers: %v", c.Indexers)

	return nil
}
