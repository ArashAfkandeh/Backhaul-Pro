package cmd

import (
	"context"
	"time"

	"github.com/musix/backhaul/internal/client"
	"github.com/musix/backhaul/internal/config"
	"github.com/musix/backhaul/internal/server"
	"github.com/musix/backhaul/internal/utils"
	"github.com/musix/backhaul/internal/web"

	"github.com/BurntSushi/toml"
)

var (
	logger = utils.NewLogger("info")
)

// Run is the main entry point for the application.
func Run(configPath string, ctx context.Context) *config.Config {
	// Store config path and ports for resilient API startup
	var webPort int = 2060 // default web/sniffer port
	var apiPort int = 2080 // default API port

	// Create a cancel function from the context (if it's cancellable)
	// If context is already cancelled, we'll create one
	_, cancel := context.WithCancel(ctx)

	// Load and parse the configuration file
	cfg, err := loadConfig(configPath)
	if err != nil {
		logger.Printf("❌ ERROR: failed to load configuration: %v", err)

		// Don't crash - try to extract ports from config for API
		// and keep API running
		cfg = &config.Config{
			Server: &config.ServerConfig{WebPort: webPort, APIPort: apiPort},
			Client: &config.ClientConfig{WebPort: webPort, APIPort: apiPort},
		}
	} else {
		// Apply default values to the configuration
		applyDefaults(cfg)

		// Extract ports from valid config
		if cfg.Server != nil {
			if cfg.Server.WebPort > 0 {
				webPort = cfg.Server.WebPort
			}
			if cfg.Server.APIPort > 0 {
				apiPort = cfg.Server.APIPort
			}
		} else if cfg.Client != nil {
			if cfg.Client.WebPort > 0 {
				webPort = cfg.Client.WebPort
			}
			if cfg.Client.APIPort > 0 {
				apiPort = cfg.Client.APIPort
			}
		}
	}

	// Start independent API server FIRST (resilient mode)
	// This ensures API is up even if main service fails to start
	logger.Printf("🔐 INFO: Starting independent API server in resilient mode (port %d)", apiPort)
	web.StartIndependentAPI(apiPort, logger, configPath, ctx, cancel)

	// Give API time to start
	time.Sleep(500 * time.Millisecond)

	// Only start tunnel services if config loaded successfully
	if err == nil {
		// Determine whether to run as a server or client
		switch {
		case cfg.Server != nil && cfg.Server.BindAddr != "":
			srv := server.NewServer(cfg.Server, ctx, configPath)

			go func() {
				srv.Start()
				<-ctx.Done()
				srv.Stop()
				logger.Println("shutting down server...")
			}()
			logger.Println("server started in background")

		case cfg.Client != nil && cfg.Client.RemoteAddr != "":
			clnt := client.NewClient(cfg.Client, ctx, configPath)

			go func() {
				clnt.Start()
				<-ctx.Done()
				clnt.Stop()
				logger.Println("shutting down client...")
			}()
			logger.Println("client started in background")

		default:
			logger.Println("WARN: neither server nor client configuration is properly set - API server only mode")
		}
	} else {
		logger.Println("WARN: Configuration invalid - running in API-only resilient mode")
		logger.Println("INFO: Use API endpoint /api/config to fix configuration")
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
