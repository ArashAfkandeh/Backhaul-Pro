package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/musix/backhaul/internal/utils"
	"github.com/musix/backhaul/internal/web"

	"github.com/musix/backhaul/internal/config"

	"github.com/musix/backhaul/internal/client/transport"

	_ "net/http/pprof"

	"github.com/sirupsen/logrus"
)

// Client encapsulates the client configuration and state
type Client struct {
	config       *config.ClientConfig
	ctx          context.Context
	cancel       context.CancelFunc
	logger       *logrus.Logger
	web          *web.Usage
	usageMonitor *web.Usage // Added for usage monitoring
}

func extractHostFromAddr(addr string) string {
	// Remove port if present (e.g., "1.2.3.4:8443" -> "1.2.3.4")
	if i := strings.LastIndex(addr, ":"); i != -1 {
		return addr[:i]
	}
	return addr
}

func (c *Client) syncKeepaliveWithServer(serverWebAddr string) {
	go func() {
		for {
			resp, err := http.Get(serverWebAddr + "/config")
			if err == nil {
				var serverCfg struct {
					Keepalive int `json:"keepalive"`
				}
				if err := json.NewDecoder(resp.Body).Decode(&serverCfg); err == nil {
					if serverCfg.Keepalive > 0 && c.config.Keepalive != serverCfg.Keepalive {
						c.logger.Infof("[SYNC] Updating client keepalive from %d to %d (server)", c.config.Keepalive, serverCfg.Keepalive)
						c.config.Keepalive = serverCfg.Keepalive
					}
				}
				resp.Body.Close()
			}
			time.Sleep(10 * time.Second)
		}
	}()
}

func (c *Client) syncConfigWithServer(serverWebAddr string) {
	go func() {
		for {
			resp, err := http.Get(serverWebAddr + "/config")
			if err == nil {
				var serverCfg struct {
					Keepalive        int `json:"keepalive_period"`
					MaxFrameSize     int `json:"mux_framesize"`
					MaxReceiveBuffer int `json:"mux_recievebuffer"`
					MaxStreamBuffer  int `json:"mux_streambuffer"`
				}
				if err := json.NewDecoder(resp.Body).Decode(&serverCfg); err == nil {
					if serverCfg.Keepalive > 0 && c.config.Keepalive != serverCfg.Keepalive {
						c.logger.Infof("[SYNC] Updating client keepalive from %d to %d (server)", c.config.Keepalive, serverCfg.Keepalive)
						c.config.Keepalive = serverCfg.Keepalive
					}
					if serverCfg.MaxFrameSize > 0 && c.config.MaxFrameSize != serverCfg.MaxFrameSize {
						c.logger.Infof("[SYNC] Updating client MaxFrameSize from %d to %d (server)", c.config.MaxFrameSize, serverCfg.MaxFrameSize)
						c.config.MaxFrameSize = serverCfg.MaxFrameSize
					}
					if serverCfg.MaxReceiveBuffer > 0 && c.config.MaxReceiveBuffer != serverCfg.MaxReceiveBuffer {
						c.logger.Infof("[SYNC] Updating client MaxReceiveBuffer from %d to %d (server)", c.config.MaxReceiveBuffer, serverCfg.MaxReceiveBuffer)
						c.config.MaxReceiveBuffer = serverCfg.MaxReceiveBuffer
					}
					if serverCfg.MaxStreamBuffer > 0 && c.config.MaxStreamBuffer != serverCfg.MaxStreamBuffer {
						c.logger.Infof("[SYNC] Updating client MaxStreamBuffer from %d to %d (server)", c.config.MaxStreamBuffer, serverCfg.MaxStreamBuffer)
						c.config.MaxStreamBuffer = serverCfg.MaxStreamBuffer
					}
				}
				resp.Body.Close()
			}
			time.Sleep(10 * time.Second)
		}
	}()
}

func NewClient(cfg *config.ClientConfig, parentCtx context.Context) *Client {
	ctx, cancel := context.WithCancel(parentCtx)
	client := &Client{
		config: cfg,
		ctx:    ctx,
		cancel: cancel,
		logger: utils.NewLogger(cfg.LogLevel),
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

	// Start keepalive sync with server web panel
	if cfg.RemoteAddr != "" && cfg.WebPort > 0 {
		host := extractHostFromAddr(cfg.RemoteAddr)
		serverWebAddr := "http://" + host + ":" + strconv.Itoa(cfg.WebPort)
		client.syncKeepaliveWithServer(serverWebAddr)
	}

	// Start config sync with server web panel
	if cfg.RemoteAddr != "" && cfg.WebPort > 0 {
		host := extractHostFromAddr(cfg.RemoteAddr)
		serverWebAddr := "http://" + host + ":" + strconv.Itoa(cfg.WebPort)
		client.syncConfigWithServer(serverWebAddr)
	}

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
			WebPort:        c.config.WebPort,
			SnifferLog:     c.config.SnifferLog,
			AggressivePool: c.config.AggressivePool,
		}
		quicClient := transport.NewQuicClient(c.ctx, quicConfig, c.logger, usageMonitor)
		go quicClient.ChannelDialer(true)
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
