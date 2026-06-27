package client

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/musix/backhaul/internal/utils"
	"github.com/musix/backhaul/internal/web"

	"github.com/musix/backhaul/internal/config"

	"github.com/musix/backhaul/internal/client/transport"

	_ "net/http/pprof"

	"github.com/BurntSushi/toml"
	"github.com/sirupsen/logrus"
)

// Client encapsulates the client configuration and state
type Client struct {
	config         *config.ClientConfig
	configFilePath string
	ctx            context.Context
	cancel         context.CancelFunc
	logger         *logrus.Logger
	web            *web.Usage
	usageMonitor   *web.Usage // Added for usage monitoring
}

func NewClient(cfg *config.ClientConfig, parentCtx context.Context, configFilePath string) *Client {
	ctx, cancel := context.WithCancel(parentCtx)
	client := &Client{
		config:         cfg,
		configFilePath: configFilePath,
		ctx:            ctx,
		cancel:         cancel,
		logger:         utils.NewLogger(cfg.LogLevel),
	}

	if cfg.ConnectionPool < 1 {
		cfg.ConnectionPool = 8
	}

	// Initialize web panel if sniffer is enabled
	sniffer := true
	if cfg.Sniffer != nil {
		sniffer = *cfg.Sniffer
	}

	var usageMonitor *web.Usage
	if sniffer && cfg.WebPort > 0 {
		tunnelStatus := "connecting"
		usageMonitor = web.NewDataStore(
			fmt.Sprintf(":%d", cfg.WebPort),
			ctx,
			cfg.SnifferLog,
			sniffer,
			&tunnelStatus,
			client.logger,
		)
		client.web = usageMonitor
		// Set config provider for web panel
		web.SetConfigProvider(client)
		// Start web panel
		go usageMonitor.Monitor()
		// Update tunnel status after a delay
		go func() {
			time.Sleep(5 * time.Second)
			tunnelStatus = "connected"
		}()
	}

	// NOTE: Do not sync with server's Sniffer/Dashboard
	// Sniffer is only accessible locally on the server
	// Client maintains its own local Sniffer instance
	// Server sync would require exposing additional APIs or port forwarding

	client.usageMonitor = usageMonitor // Add this field to Client struct if not present

	return client
}

// Run starts the client and begins dialing the tunnel server
func (c *Client) Start() {
	// for pprof
	if c.config.PPROF {
		go func() {
			c.logger.Info("pprof started at port 6061")
			http.ListenAndServe("0.0.0.0:6061", nil)
		}()
	}

	c.logger.Infof("client with remote address %s started successfully", c.config.RemoteAddr)

	sniffer := true
	if c.config.Sniffer != nil {
		sniffer = *c.config.Sniffer
	}

	usageMonitor := c.usageMonitor

	switch c.config.Transport {
	case config.TCP:
		tcpConfig := &transport.TcpConfig{
			RemoteAddr:     c.config.RemoteAddr,
			Nodelay:        c.config.Nodelay,
			KeepAlive:      time.Duration(c.config.Keepalive) * time.Second,
			RetryInterval:  time.Duration(c.config.RetryInterval) * time.Second,
			DialTimeOut:    time.Duration(c.config.DialTimeout) * time.Second,
			ConnPoolSize:   c.config.ConnectionPool,
			Token:          c.config.Token,
			Sniffer:        sniffer,
			WebPort:        c.config.WebPort,
			SnifferLog:     c.config.SnifferLog,
			AggressivePool: c.config.AggressivePool,
			TunnelMode:     c.config.TunnelMode,
			Ports:          c.config.Ports,
			AcceptUDP:      c.config.AcceptUDP,
		}
		tcpClient := transport.NewTCPClient(c.ctx, tcpConfig, c.logger, usageMonitor)
		go tcpClient.Start()

	case config.TCPMUX:
		tcpMuxConfig := &transport.TcpMuxConfig{
			RemoteAddr:       c.config.RemoteAddr,
			Nodelay:          c.config.Nodelay,
			KeepAlive:        time.Duration(c.config.Keepalive) * time.Second,
			RetryInterval:    time.Duration(c.config.RetryInterval) * time.Second,
			DialTimeOut:      time.Duration(c.config.DialTimeout) * time.Second,
			ConnPoolSize:     c.config.ConnectionPool,
			Token:            c.config.Token,
			MuxVersion:       c.config.MuxVersion,
			MaxFrameSize:     c.config.MaxFrameSize,
			MaxReceiveBuffer: c.config.MaxReceiveBuffer,
			MaxStreamBuffer:  c.config.MaxStreamBuffer,
			Sniffer:          sniffer,
			WebPort:          c.config.WebPort,
			SnifferLog:       c.config.SnifferLog,
			AggressivePool:   c.config.AggressivePool,
			TunnelMode:       c.config.TunnelMode,
			Ports:            c.config.Ports,
			AcceptUDP:        c.config.AcceptUDP,
		}
		tcpMuxClient := transport.NewMuxClient(c.ctx, tcpMuxConfig, c.logger, usageMonitor)
		go tcpMuxClient.Start()

	case config.WS, config.WSS:
		WsConfig := &transport.WsConfig{
			RemoteAddr:     c.config.RemoteAddr,
			Nodelay:        c.config.Nodelay,
			KeepAlive:      time.Duration(c.config.Keepalive) * time.Second,
			RetryInterval:  time.Duration(c.config.RetryInterval) * time.Second,
			DialTimeOut:    time.Duration(c.config.DialTimeout) * time.Second,
			ConnPoolSize:   c.config.ConnectionPool,
			Token:          c.config.Token,
			Sniffer:        sniffer,
			WebPort:        c.config.WebPort,
			SnifferLog:     c.config.SnifferLog,
			Mode:           c.config.Transport,
			AggressivePool: c.config.AggressivePool,
			EdgeIP:         c.config.EdgeIP,
			TunnelMode:     c.config.TunnelMode,
			Ports:          c.config.Ports,
			AcceptUDP:      c.config.AcceptUDP,
		}
		WsClient := transport.NewWSClient(c.ctx, WsConfig, c.logger, usageMonitor)
		go WsClient.Start()

	case config.WSMUX, config.WSSMUX:
		wsMuxConfig := &transport.WsMuxConfig{
			RemoteAddr:       c.config.RemoteAddr,
			Nodelay:          c.config.Nodelay,
			KeepAlive:        time.Duration(c.config.Keepalive) * time.Second,
			RetryInterval:    time.Duration(c.config.RetryInterval) * time.Second,
			DialTimeOut:      time.Duration(c.config.DialTimeout) * time.Second,
			ConnPoolSize:     c.config.ConnectionPool,
			Token:            c.config.Token,
			MuxVersion:       c.config.MuxVersion,
			MaxFrameSize:     c.config.MaxFrameSize,
			MaxReceiveBuffer: c.config.MaxReceiveBuffer,
			MaxStreamBuffer:  c.config.MaxStreamBuffer,
			Sniffer:          sniffer,
			WebPort:          c.config.WebPort,
			SnifferLog:       c.config.SnifferLog,
			Mode:             c.config.Transport,
			AggressivePool:   c.config.AggressivePool,
			EdgeIP:           c.config.EdgeIP,
			TunnelMode:       c.config.TunnelMode,
			Ports:            c.config.Ports,
			AcceptUDP:        c.config.AcceptUDP,
		}
		wsMuxClient := transport.NewWSMuxClient(c.ctx, wsMuxConfig, c.logger, usageMonitor)
		go wsMuxClient.Start()

	case config.QUIC:
		quicConfig := &transport.QuicConfig{
			RemoteAddr:     c.config.RemoteAddr,
			Nodelay:        c.config.Nodelay,
			KeepAlive:      time.Duration(c.config.Keepalive) * time.Second,
			RetryInterval:  time.Duration(c.config.RetryInterval) * time.Second,
			DialTimeOut:    time.Duration(c.config.DialTimeout) * time.Second,
			ConnectionPool: c.config.ConnectionPool,
			Token:          c.config.Token,
			Sniffer:        sniffer,
			SnifferPort:    c.config.WebPort,
			SnifferLog:     c.config.SnifferLog,
			AggressivePool: c.config.AggressivePool,
			TunnelMode:     c.config.TunnelMode,
			Ports:          c.config.Ports,
			AcceptUDP:      c.config.AcceptUDP,
		}
		quicClient := transport.NewQuicClient(c.ctx, quicConfig, c.logger, usageMonitor)
		go quicClient.ChannelDialer(true)

	case config.UDP:
		udpConfig := &transport.UdpConfig{
			RemoteAddr:     c.config.RemoteAddr,
			Token:          c.config.Token,
			SnifferLog:     c.config.SnifferLog,
			RetryInterval:  time.Duration(c.config.RetryInterval) * time.Second,
			DialTimeOut:    time.Duration(c.config.DialTimeout) * time.Second,
			ConnPoolSize:   c.config.ConnectionPool,
			SnifferPort:    c.config.WebPort,
			Sniffer:        sniffer,
			AggressivePool: c.config.AggressivePool,
			TunnelMode:     c.config.TunnelMode,
			Ports:          c.config.Ports,
		}
		udpClient := transport.NewUDPClient(c.ctx, udpConfig, c.logger, usageMonitor)
		go udpClient.Start()
	}

	<-c.ctx.Done()

	c.logger.Info("all workers stopped successfully")

	// suppress other logs
	c.logger.SetLevel(logrus.FatalLevel)
}
func (c *Client) Stop() {
	if c.cancel != nil {
		c.cancel()
	}
}

// GetServerConfig implements web.ConfigProvider interface
func (c *Client) GetServerConfig() *config.ServerConfig {
	return nil // Client doesn't have server config
}

// GetClientConfig implements web.ConfigProvider interface
func (c *Client) GetClientConfig() *config.ClientConfig {
	// c.logger.Info("[TEST] GetClientConfig called, keepalive=", c.config.Keepalive)
	return c.config
}

// GetConfigFilePath implements web.ConfigProvider interface
func (c *Client) GetConfigFilePath() string {
	return c.configFilePath
}

// SaveConfig implements web.ConfigProvider interface
func (c *Client) SaveConfig() error {
	// Marshal the full config back to TOML
	fullConfig := &config.Config{
		Client: c.config,
	}

	// Write to file
	f, err := os.Create(c.configFilePath)
	if err != nil {
		return err
	}
	defer f.Close()

	encoder := toml.NewEncoder(f)
	return encoder.Encode(fullConfig)
}

// SaveConfigUpdates implements web.ConfigProvider interface - updates only changed fields
func (c *Client) SaveConfigUpdates(updates map[string]interface{}, configType string) error {
	if configType != "client" {
		return fmt.Errorf("client provider can only save client config updates")
	}

	for key, value := range updates {
		if err := utils.UpdateTOMLValue(c.configFilePath, "client", key, value); err != nil {
			return err
		}
	}
	return nil
}
