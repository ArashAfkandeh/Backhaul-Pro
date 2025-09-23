package cmd

import (
	"context"

	"github.com/musix/backhaul/internal/client"
	"github.com/musix/backhaul/internal/config"
	"github.com/musix/backhaul/internal/server"
	"github.com/musix/backhaul/internal/utils"

	"github.com/BurntSushi/toml"
)

var (
	logger = utils.NewLogger("info")
)

// Run is the main entry point for the application.
func Run(configPath string, ctx context.Context) *config.Config {
	// Load and parse the configuration file
	cfg, err := loadConfig(configPath)
	if err != nil {
		logger.Fatalf("failed to load configuration: %v", err)
	}

	// Apply default values to the configuration
	applyDefaults(cfg)

	// Determine whether to run as a server or client
	switch {
	case cfg.Server != nil && cfg.Server.BindAddr != "":
		srv := server.NewServer(cfg.Server, ctx)
		go func() {
			srv.Start()
			<-ctx.Done()
			srv.Stop()
			logger.Println("shutting down server...")
		}()
		logger.Println("server started in background")

	case cfg.Client != nil && cfg.Client.RemoteAddr != "":
		clnt := client.NewClient(cfg.Client, ctx)
		go func() {
			clnt.Start()
			<-ctx.Done()
			clnt.Stop()
			logger.Println("shutting down client...")
		}()
		logger.Println("client started in background")

	default:
		logger.Fatalf("neither server nor client configuration is properly set.")
	}

	return cfg // Return the config object immediately
}

// loadConfig loads and parses the TOML configuration file.
func loadConfig(configPath string) (*config.Config, error) {
	var cfg config.Config
	if _, err := toml.DecodeFile(configPath, &cfg); err != nil {
		return &cfg, err
	}
	// مقداردهی پیش‌فرض اگر nil بود
	if cfg.Server == nil {
		cfg.Server = &config.ServerConfig{}
	}
	if cfg.Client == nil {
		cfg.Client = &config.ClientConfig{}
	}
	return &cfg, nil
}
