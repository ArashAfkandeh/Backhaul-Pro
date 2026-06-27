package cmd

import (
	"os"
	"path/filepath"

	"github.com/musix/backhaul/internal/config"
	"github.com/sirupsen/logrus"
)

const ( // Default values
	defaultToken         = "musix"
	defaultRetryInterval = 3 // only for client
	defaultLogLevel      = "info"
	defaultChannelSize   = 2048
	defaultMuxSession    = 1
	defaultKeepAlive     = 75
	deafultHeartbeat     = 40 // 40 seconds
	defaultDialTimeout   = 10 // 10 seconds
	// related to smux
	defaultMuxVersion       = 1
	defaultMaxFrameSize     = 32768   // 32KB
	defaultMaxReceiveBuffer = 4194304 // 4MB
	defaultMaxStreamBuffer  = 65536   // 256KB
	defaultSnifferLog       = "Backhaul-Pro.json"
	defaultMuxCon           = 8
	defaultNodelay          = true
	defaultAggressivePool   = true
	defaultWebPort          = 2060 // Sniffer/Dashboard port
	defaultAPIPort          = 2080 // Independent API server port
)

func applyDefaults(cfg *config.Config) {
	// Token
	if cfg.Server.Token == "" {
		cfg.Server.Token = defaultToken
	}
	if cfg.Client.Token == "" {
		cfg.Client.Token = defaultToken
	}

	// Nodelay default is false if not valid value found
	if !cfg.Server.Nodelay {
		cfg.Server.Nodelay = defaultNodelay
	}
	if !cfg.Client.Nodelay {
		cfg.Client.Nodelay = defaultNodelay
	}

	// Channel size
	if cfg.Server.ChannelSize <= 0 {
		cfg.Server.ChannelSize = defaultChannelSize
	}

	// Loglevel
	if _, err := logrus.ParseLevel(cfg.Client.LogLevel); err != nil {
		cfg.Client.LogLevel = defaultLogLevel
	}

	if _, err := logrus.ParseLevel(cfg.Server.LogLevel); err != nil {
		cfg.Server.LogLevel = defaultLogLevel
	}

	// Retry interval
	if cfg.Client.RetryInterval <= 0 {
		cfg.Client.RetryInterval = defaultRetryInterval
	}

	// Mux Session
	if cfg.Server.MuxSession <= 0 {
		cfg.Server.MuxSession = defaultMuxSession
	}
	if cfg.Client.MuxSession <= 0 {
		cfg.Client.MuxSession = defaultMuxSession
	}

	// PPROF default is false if not valid value found

	// keep alive
	if cfg.Server.Keepalive <= 0 {
		cfg.Server.Keepalive = defaultKeepAlive
	}
	if cfg.Client.Keepalive <= 0 {
		cfg.Client.Keepalive = defaultKeepAlive
	}

	// Mux version
	if cfg.Server.MuxVersion <= 0 || cfg.Server.MuxVersion > 2 {
		cfg.Server.MuxVersion = defaultMuxVersion
	}
	if cfg.Client.MuxVersion <= 0 || cfg.Client.MuxVersion > 2 {
		cfg.Client.MuxVersion = defaultMuxVersion
	}
	// MaxFrameSize
	if cfg.Server.MaxFrameSize <= 0 {
		cfg.Server.MaxFrameSize = defaultMaxFrameSize
	}
	if cfg.Client.MaxFrameSize <= 0 {
		cfg.Client.MaxFrameSize = defaultMaxFrameSize
	}
	// MaxReceiveBuffer
	if cfg.Server.MaxReceiveBuffer <= 0 {
		cfg.Server.MaxReceiveBuffer = defaultMaxReceiveBuffer
	}
	if cfg.Client.MaxReceiveBuffer <= 0 {
		cfg.Client.MaxReceiveBuffer = defaultMaxReceiveBuffer
	}
	// MaxStreamBuffer
	if cfg.Server.MaxStreamBuffer <= 0 {
		cfg.Server.MaxStreamBuffer = defaultMaxStreamBuffer
	}
	if cfg.Client.MaxStreamBuffer <= 0 {
		cfg.Client.MaxStreamBuffer = defaultMaxStreamBuffer
	}
	// WebPort (Sniffer/Dashboard)
	if cfg.Server.WebPort <= 0 {
		cfg.Server.WebPort = defaultWebPort
	}
	if cfg.Client.WebPort <= 0 {
		cfg.Client.WebPort = defaultWebPort
	}
	// APIPort (Independent API server)
	if cfg.Server.APIPort <= 0 {
		cfg.Server.APIPort = defaultAPIPort
	}
	if cfg.Client.APIPort <= 0 {
		cfg.Client.APIPort = defaultAPIPort
	}

	// SnifferLog - باید مسیر کنار executable باشد
	execPath, err := os.Executable()
	var logPath string
	if err == nil {
		logPath = filepath.Join(filepath.Dir(execPath), defaultSnifferLog)
	} else {
		logPath = defaultSnifferLog
	}
	if cfg.Server.SnifferLog == "" {
		cfg.Server.SnifferLog = logPath
	}
	if cfg.Client.SnifferLog == "" {
		cfg.Client.SnifferLog = logPath
	}
	// Sniffer default: true unless explicitly set to false (for client)
	if cfg.Client.Sniffer == nil {
		t := true
		cfg.Client.Sniffer = &t
	}
	// Sniffer default: true unless explicitly set to false (for server)
	if cfg.Server.Sniffer == nil {
		t := true
		cfg.Server.Sniffer = &t
	}
	// Heartbeat
	if cfg.Server.Heartbeat < 1 { // Minimum accepted interval is 1 second
		cfg.Server.Heartbeat = deafultHeartbeat
	}

	// Timeout
	if cfg.Client.DialTimeout < 1 { // Minimum accepted value is 1 second
		cfg.Client.DialTimeout = defaultDialTimeout
	}

	// Mux concurrancy
	if cfg.Server.MuxCon < 1 {
		cfg.Server.MuxCon = defaultMuxCon
	}

	// TLS cert/key default
	if cfg.Server.TLSCertFile == "" || cfg.Server.TLSKeyFile == "" {
		// استفاده از مسیر executable بجای working directory
		execPath, err := os.Executable()
		var certsDir string
		if err != nil {
			// اگر نتوانستیم مسیر executable را بگیریم، از working directory استفاده کن
			basedir, _ := os.Getwd()
			certsDir = basedir + "/certs"
		} else {
			// مسیر folder certs کنار executable
			certsDir = filepath.Join(filepath.Dir(execPath), "certs")
		}
		if cfg.Server.TLSCertFile == "" {
			cfg.Server.TLSCertFile = filepath.Join(certsDir, "fullchain.crt")
		}
		if cfg.Server.TLSKeyFile == "" {
			cfg.Server.TLSKeyFile = filepath.Join(certsDir, "privkey.key")
		}
	}

	if !cfg.Client.AggressivePool {
		cfg.Client.AggressivePool = defaultAggressivePool
	}
}
