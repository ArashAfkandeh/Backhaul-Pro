package server

import (
	"context"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"time"

	"github.com/musix/backhaul/internal/config"
	"github.com/musix/backhaul/internal/server/transport"
	"github.com/musix/backhaul/internal/utils"
	"github.com/musix/backhaul/internal/web"

	"github.com/BurntSushi/toml"
	"github.com/sirupsen/logrus"
)

type Server struct {
	config         *config.ServerConfig
	configFilePath string
	ctx            context.Context
	cancel         context.CancelFunc
	logger         *logrus.Logger
}

// پیاده‌سازی ConfigProvider
func (s *Server) GetServerConfig() *config.ServerConfig {
	return s.config
}

// GetClientConfig implements web.ConfigProvider interface
func (s *Server) GetClientConfig() *config.ClientConfig {
	return nil // Server doesn't have client config
}

// GetConfigFilePath implements web.ConfigProvider interface
func (s *Server) GetConfigFilePath() string {
	return s.configFilePath
}

// SaveConfig implements web.ConfigProvider interface
func (s *Server) SaveConfig() error {
	// Marshal the full config back to TOML
	fullConfig := &config.Config{
		Server: s.config,
	}

	// Write to file
	f, err := os.Create(s.configFilePath)
	if err != nil {
		return err
	}
	defer f.Close()

	encoder := toml.NewEncoder(f)
	return encoder.Encode(fullConfig)
}

// SaveConfigUpdates implements web.ConfigProvider interface - updates only changed fields
func (s *Server) SaveConfigUpdates(updates map[string]interface{}, configType string) error {
	if configType != "server" {
		return fmt.Errorf("server provider can only save server config updates")
	}

	for key, value := range updates {
		if err := utils.UpdateTOMLValue(s.configFilePath, "server", key, value); err != nil {
			return err
		}
	}
	return nil
}

func NewServer(cfg *config.ServerConfig, parentCtx context.Context, configFilePath string) *Server {
	ctx, cancel := context.WithCancel(parentCtx)

	if cfg.ChannelSize < 1 {
		cfg.ChannelSize = 8
	}

	return &Server{
		config:         cfg,
		configFilePath: configFilePath,
		ctx:            ctx,
		cancel:         cancel,
		logger:         utils.NewLogger(cfg.LogLevel),
	}
}

func (s *Server) Start() {
	// ثبت provider برای web panel
	web.SetConfigProvider(s)
	// for pprof and debugging
	if s.config.PPROF {
		go func() {
			s.logger.Info("pprof started at port 6060")
			http.ListenAndServe("0.0.0.0:6060", nil)
		}()
	}

	switch s.config.Transport {
	case config.TCP:
		tcpConfig := &transport.TcpConfig{
			BindAddr:       s.config.BindAddr,
			Nodelay:        s.config.Nodelay,
			KeepAlive:      time.Duration(s.config.Keepalive) * time.Second,
			Heartbeat:      time.Duration(s.config.Heartbeat) * time.Second,
			Token:          s.config.Token,
			ChannelSize:    s.config.ChannelSize,
			Ports:          s.config.Ports,
			Sniffer:        *s.config.Sniffer,
			WebPort:        s.config.WebPort,
			SnifferLog:     s.config.SnifferLog,
			TunnelMode:     s.config.TunnelMode,
			AcceptUDP:      s.config.AcceptUDP,
			AllowedClients: s.config.AllowedClients,
		}

		tcpServer := transport.NewTCPServer(s.ctx, tcpConfig, s.logger)
		go tcpServer.Start()

	case config.TCPMUX:
		tcpMuxConfig := &transport.TcpMuxConfig{
			BindAddr:         s.config.BindAddr,
			Nodelay:          s.config.Nodelay,
			KeepAlive:        time.Duration(s.config.Keepalive) * time.Second,
			Heartbeat:        time.Duration(s.config.Heartbeat) * time.Second,
			Token:            s.config.Token,
			ChannelSize:      s.config.ChannelSize,
			Ports:            s.config.Ports,
			MuxCon:           s.config.MuxCon,
			MuxVersion:       s.config.MuxVersion,
			MaxFrameSize:     s.config.MaxFrameSize,
			MaxReceiveBuffer: s.config.MaxReceiveBuffer,
			MaxStreamBuffer:  s.config.MaxStreamBuffer,
			Sniffer:          *s.config.Sniffer,
			WebPort:          s.config.WebPort,
			SnifferLog:       s.config.SnifferLog,
			AllowedClients:   s.config.AllowedClients,
			TunnelMode:       s.config.TunnelMode,
			AcceptUDP:        s.config.AcceptUDP,
		}

		tcpMuxServer := transport.NewTcpMuxServer(s.ctx, tcpMuxConfig, s.logger)
		go tcpMuxServer.Start()

	case config.WS, config.WSS:
		wsConfig := &transport.WsConfig{
			BindAddr:       s.config.BindAddr,
			Nodelay:        s.config.Nodelay,
			KeepAlive:      time.Duration(s.config.Keepalive) * time.Second,
			Heartbeat:      time.Duration(s.config.Heartbeat) * time.Second,
			Token:          s.config.Token,
			ChannelSize:    s.config.ChannelSize,
			Ports:          s.config.Ports,
			Sniffer:        *s.config.Sniffer,
			WebPort:        s.config.WebPort,
			SnifferLog:     s.config.SnifferLog,
			Mode:           s.config.Transport,
			TLSCertFile:    s.config.TLSCertFile,
			TLSKeyFile:     s.config.TLSKeyFile,
			AllowedClients: s.config.AllowedClients,
			TunnelMode:     s.config.TunnelMode,
			AcceptUDP:      s.config.AcceptUDP,
		}

		wsServer := transport.NewWSServer(s.ctx, wsConfig, s.logger)
		go wsServer.Start()

	case config.WSMUX, config.WSSMUX:
		wsMuxConfig := &transport.WsMuxConfig{
			BindAddr:         s.config.BindAddr,
			Nodelay:          s.config.Nodelay,
			KeepAlive:        time.Duration(s.config.Keepalive) * time.Second,
			Heartbeat:        time.Duration(s.config.Heartbeat) * time.Second,
			Token:            s.config.Token,
			ChannelSize:      s.config.ChannelSize,
			Ports:            s.config.Ports,
			MuxCon:           s.config.MuxCon,
			MuxVersion:       s.config.MuxVersion,
			MaxFrameSize:     s.config.MaxFrameSize,
			MaxReceiveBuffer: s.config.MaxReceiveBuffer,
			MaxStreamBuffer:  s.config.MaxStreamBuffer,
			Sniffer:          *s.config.Sniffer,
			WebPort:          s.config.WebPort,
			SnifferLog:       s.config.SnifferLog,
			Mode:             s.config.Transport,
			TLSCertFile:      s.config.TLSCertFile,
			TLSKeyFile:       s.config.TLSKeyFile,
			AllowedClients:   s.config.AllowedClients,
			TunnelMode:       s.config.TunnelMode,
			AcceptUDP:        s.config.AcceptUDP,
		}

		wsMuxServer := transport.NewWSMuxServer(s.ctx, wsMuxConfig, s.logger)
		go wsMuxServer.Start()

	case config.QUIC:
		quicConfig := &transport.QuicConfig{
			BindAddr:       s.config.BindAddr,
			Nodelay:        s.config.Nodelay,
			KeepAlive:      time.Duration(s.config.Keepalive) * time.Second,
			Heartbeat:      time.Duration(s.config.Heartbeat) * time.Second,
			Token:          s.config.Token,
			MuxCon:         s.config.MuxCon,
			ChannelSize:    s.config.ChannelSize,
			Ports:          s.config.Ports,
			Sniffer:        *s.config.Sniffer,
			WebPort:        s.config.WebPort,
			SnifferLog:     s.config.SnifferLog,
			TLSCertFile:    s.config.TLSCertFile,
			TLSKeyFile:     s.config.TLSKeyFile,
			AllowedClients: s.config.AllowedClients,
			TunnelMode:     s.config.TunnelMode,
			AcceptUDP:      s.config.AcceptUDP,
		}

		quicServer := transport.NewQuicServer(s.ctx, quicConfig, s.logger)
		go quicServer.TunnelListener()

	case config.UDP:
		udpConfig := &transport.UdpConfig{
			BindAddr:       s.config.BindAddr,
			Heartbeat:      time.Duration(s.config.Heartbeat) * time.Second,
			Token:          s.config.Token,
			ChannelSize:    s.config.ChannelSize,
			Ports:          s.config.Ports,
			Sniffer:        *s.config.Sniffer,
			WebPort:        s.config.WebPort,
			SnifferLog:     s.config.SnifferLog,
			AllowedClients: s.config.AllowedClients,
			TunnelMode:     s.config.TunnelMode,
		}

		udpServer := transport.NewUDPServer(s.ctx, udpConfig, s.logger)
		go udpServer.Start()

	default:
		s.logger.Fatal("invalid transport type: ", s.config.Transport)
	}

	<-s.ctx.Done()

	s.logger.Info("all workers stopped successfully")

	// suppress other logs
	s.logger.SetLevel(logrus.FatalLevel)
}

// Stop shuts down the server gracefully
func (s *Server) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
}
