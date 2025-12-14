package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	arcadeconfig "github.com/bsv-blockchain/arcade/config"
	chaintracksconfig "github.com/bsv-blockchain/go-chaintracks/config"
	p2p "github.com/bsv-blockchain/go-teranode-p2p-client"
	ordfsconfig "github.com/shruggr/go-ordfs-server/config"
	"github.com/spf13/viper"
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
		Type      string `mapstructure:"type"`      // main, test
		JungleBus string `mapstructure:"junglebus"` // JungleBus URL
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

	// P2P client configuration
	P2P p2p.Config `mapstructure:"p2p"`

	// Chaintracks configuration (separate service)
	Chaintracks chaintracksconfig.Config `mapstructure:"chaintracks"`

	// Arcade configuration (receives Chaintracks instance)
	Arcade arcadeconfig.Config `mapstructure:"arcade"`

	// ORDFS configuration (optional - enables content serving)
	Ordfs ordfsconfig.Config `mapstructure:"ordfs"`

	// BSV21 configuration (optional - enables BSV21 token routes)
	BSV21 struct {
		Enabled   bool   `mapstructure:"enabled"`
		EventsURL string `mapstructure:"events_url"` // SQLite/PostgreSQL/MongoDB URL for BSV21 events
	} `mapstructure:"bsv21"`
}

// SetDefaults sets viper defaults for indexer configuration.
// When used as an embedded library, pass a prefix to namespace the config.
func (c *Config) SetDefaults(v *viper.Viper, prefix string) {
	p := ""
	if prefix != "" {
		p = prefix + "."
	}

	// Store defaults
	v.SetDefault(p+"store.type", "sqlite")
	v.SetDefault(p+"store.sqlite", "~/.1sat/indexer.db")

	// PubSub defaults
	v.SetDefault(p+"pubsub.url", "channels://")

	// BEEF defaults
	v.SetDefault(p+"beef.url", "lru://?size=100mb,~/.1sat/beef,junglebus://")

	// Network defaults
	v.SetDefault(p+"network.type", "main")
	v.SetDefault(p+"network.junglebus", "https://junglebus.gorillapool.io")

	// Server defaults
	v.SetDefault(p+"server.port", 8080)

	// Indexer defaults
	v.SetDefault(p+"indexers", []string{"p2pkh", "lock", "inscription", "ordlock"})

	// Cascade to P2P defaults
	c.P2P.SetDefaults(v, p+"p2p")

	// Cascade to Chaintracks defaults
	c.Chaintracks.SetDefaults(v, p+"chaintracks")

	// Cascade to arcade defaults
	c.Arcade.SetDefaults(v, p+"arcade")

	// Cascade to ORDFS defaults
	c.Ordfs.SetDefaults(v, p+"ordfs")

	// BSV21 defaults (disabled by default)
	v.SetDefault(p+"bsv21.enabled", false)
	v.SetDefault(p+"bsv21.events_url", "~/.1sat/bsv21.db")
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
	cfg := &Config{}

	// Set defaults using the new pattern
	cfg.SetDefaults(v, "")

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


	// Read config file (optional - env vars can provide everything)
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("error reading config file: %w", err)
		}
		// Config file not found is OK - use defaults and env vars
	}

	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("error unmarshaling config: %w", err)
	}

	// Expand ~ in paths
	cfg.Store.SQLite = expandPath(cfg.Store.SQLite)
	cfg.Beef.URL = expandBeefPath(cfg.Beef.URL)
	cfg.BSV21.EventsURL = expandPath(cfg.BSV21.EventsURL)

	return cfg, nil
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
