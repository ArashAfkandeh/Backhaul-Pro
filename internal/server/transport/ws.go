package transport

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/musix/backhaul/internal/config"
	"github.com/musix/backhaul/internal/utils"
	"github.com/musix/backhaul/internal/web"
	"github.com/musix/backhaul/internal/client/transport"

	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
)

type WsTransport struct {
	config         *WsConfig
	parentctx      context.Context
	ctx            context.Context
	cancel         context.CancelFunc
	logger         *logrus.Logger
	tunnelChannel  chan TunnelChannel
	localChannel   chan LocalTCPConn
	reqNewConnChan chan struct{}
	controlChannel *websocket.Conn
	restartMutex   sync.Mutex
	usageMonitor   *web.Usage
}

type WsConfig struct {
	BindAddr       string
	SnifferLog     string
	TLSCertFile    string // Path to the TLS certificate file
	TLSKeyFile     string // Path to the TLS key file
	TunnelStatus   string
	Token          string
	Ports          []string
	Nodelay        bool
	Sniffer        bool
	KeepAlive      time.Duration
	Heartbeat      time.Duration // in seconds
	ChannelSize    int
	WebPort        int
	Mode           config.TransportType // ws or wss
	AllowedClients []string             // Whitelist of allowed client IPs/domains
	TunnelMode     string
	AcceptUDP      bool
}

func NewWSServer(parentCtx context.Context, config *WsConfig, logger *logrus.Logger) *WsTransport {
	// Create a derived context from the parent context
	ctx, cancel := context.WithCancel(parentCtx)

	// Initialize the TcpTransport struct
	server := &WsTransport{
		config:         config,
		parentctx:      parentCtx,
		ctx:            ctx,
		cancel:         cancel,
		logger:         logger,
		tunnelChannel:  make(chan TunnelChannel, config.ChannelSize),
		localChannel:   make(chan LocalTCPConn, config.ChannelSize),
		reqNewConnChan: make(chan struct{}, config.ChannelSize),
		controlChannel: nil, // will be set when a control connection is established
		usageMonitor:   web.NewDataStore(fmt.Sprintf(":%v", config.WebPort), ctx, config.SnifferLog, config.Sniffer, &config.TunnelStatus, logger),
	}

	return server
}

func (s *WsTransport) Start() {
	// for  webui
	if s.config.WebPort > 0 {
		go s.usageMonitor.Monitor()
	}

	s.config.TunnelStatus = fmt.Sprintf("Disconnected (%s)", s.config.Mode)

	go s.tunnelListener()

}
func (s *WsTransport) Restart() {
	if !s.restartMutex.TryLock() {
		s.logger.Warn("server restart already in progress, skipping restart attempt")
		return
	}
	defer s.restartMutex.Unlock()

	s.logger.Info("restarting server...")

	level := s.logger.Level
	s.logger.SetLevel(logrus.FatalLevel)

	if s.cancel != nil {
		s.cancel()
	}

	// Close control channel connection
	if s.controlChannel != nil {
		s.controlChannel.Close()
	}

	time.Sleep(2 * time.Second)

	ctx, cancel := context.WithCancel(s.parentctx)
	s.ctx = ctx
	s.cancel = cancel

	// Re-initialize variables
	s.tunnelChannel = make(chan TunnelChannel, s.config.ChannelSize)
	s.localChannel = make(chan LocalTCPConn, s.config.ChannelSize)
	s.reqNewConnChan = make(chan struct{}, s.config.ChannelSize)
	s.controlChannel = nil
	s.usageMonitor = web.NewDataStore(fmt.Sprintf(":%v", s.config.WebPort), ctx, s.config.SnifferLog, s.config.Sniffer, &s.config.TunnelStatus, s.logger)
	s.config.TunnelStatus = ""

	// set the log level again
	s.logger.SetLevel(level)

	go s.Start()
}

func (s *WsTransport) channelHandler() {
	const maxRetries = 3
	const baseBackoff = time.Second
	ticker := time.NewTicker(s.config.Heartbeat)
	defer ticker.Stop()
	messageChan := make(chan byte, 10)

	// Separate goroutine to continuously listen for messages with retry/backoff
	go func() {
		retries := 0
		for {
			select {
			case <-s.ctx.Done():
				return
			default:
				_, msg, err := s.controlChannel.ReadMessage()
				if err != nil {
					s.logger.Errorf("failed to read from channel connection (try %d/%d): %v", retries+1, maxRetries, err)
					retries++
					if retries >= maxRetries {
						if s.cancel != nil {
							s.logger.Error("max retries reached, restarting...")
							go s.Restart()
						}
						return
					}
					time.Sleep(baseBackoff * time.Duration(retries))
					continue
				}
				retries = 0 // reset on success
				if len(msg) == 0 {
					s.logger.Warn("received empty message from control channel")
					continue
				}
				messageChan <- msg[0]
			}
		}
	}()

	for {
		select {
		case <-s.ctx.Done():
			_ = s.controlChannel.WriteMessage(websocket.BinaryMessage, []byte{utils.SG_Closed})
			return
		case <-s.reqNewConnChan:
			err := s.controlChannel.WriteMessage(websocket.BinaryMessage, []byte{utils.SG_Chan})
			if err != nil {
				s.logger.Error("failed to send request new connection signal. ", err)
				go s.Restart()
				return
			}

		case <-ticker.C:
			err := s.controlChannel.WriteMessage(websocket.BinaryMessage, []byte{utils.SG_HB})
			if err != nil {
				s.logger.Errorf("failed to send heartbeat signal. Error: %v.", err)
				go s.Restart()
				return
			}
			s.logger.Debug("heartbeat signal sent successfully")

		case msg, ok := <-messageChan:
			if !ok {
				s.logger.Error("channel closed, likely due to an error in WebSocket read")
				return
			}
			switch msg {
			case utils.SG_HB:
				s.logger.Trace("heartbeat signal received successfully")

			case utils.SG_Closed:
				s.logger.Warn("control channel has been closed by the client")
				s.Restart()
				return

			default:
				s.logger.Errorf("unexpected response from channel: %v", msg)
				go s.Restart()
				return
			}

		}
	}
}

func (s *WsTransport) tunnelListener() {
	addr := s.config.BindAddr
	upgrader := websocket.Upgrader{
		ReadBufferSize:   16 * 1024,
		WriteBufferSize:  16 * 1024,
		HandshakeTimeout: 45 * time.Second,
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}

	// Create an HTTP server
	server := &http.Server{
		Addr:        addr,
		IdleTimeout: -1,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			s.logger.Tracef("received http request from %s", r.RemoteAddr)

			// Check if client is in whitelist
			if !utils.IsClientAllowed(r.RemoteAddr, s.config.AllowedClients) {
				s.logger.Warnf("client %s is not in whitelist, rejecting connection", r.RemoteAddr)
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}

			// Read the "Authorization" header
			authHeader := r.Header.Get("Authorization")
			if authHeader != fmt.Sprintf("Bearer %v", s.config.Token) {
				s.logger.Warnf("unauthorized request from %s, closing connection", r.RemoteAddr)
				http.Error(w, "unauthorized", http.StatusUnauthorized) // Send 401 Unauthorized response
				return
			}

			// Handle obfuscated paths - extract the actual path
			requestPath := r.URL.Path
			s.logger.Tracef("received request for path: %s", requestPath)

			// Check if this is an obfuscated path (contains realistic path patterns)
			obfuscatedPaths := []string{
				"/api/v1/stream", "/cdn/assets", "/ws/chat", "/api/notifications",
				"/live/stream", "/api/analytics", "/cdn/static", "/api/status",
				"/ws/updates", "/api/metrics",
			}

			// Check if this is an obfuscated control channel path
			isObfuscatedControl := false
			for _, obfPath := range obfuscatedPaths {
				if strings.HasPrefix(requestPath, obfPath) {
					s.logger.Tracef("detected obfuscated path: %s", requestPath)
					// Check if this is a control channel (contains user ID)
					if strings.Contains(requestPath, "/") && len(strings.Split(requestPath, "/")) >= 3 {
						isObfuscatedControl = true
					}
					break
				}
			}

			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				s.logger.Errorf("failed to upgrade connection from %s: %v", r.RemoteAddr, err)
				return
			}

			// Handle control channel (both normal and obfuscated)
			if r.URL.Path == "/channel" || isObfuscatedControl {
				if s.controlChannel != nil {
					s.logger.Warn("new control channel requested.")
					s.controlChannel.Close()
					conn.Close()
					go s.Restart()
					return
				}
				s.controlChannel = conn

				s.logger.Info("control channel established successfully")

				numCPU := runtime.NumCPU()
				if numCPU > 4 {
					numCPU = 4 // Max allowed handler is 4
				}

				if s.config.TunnelMode == "direct" {
					go s.channelHandlerDirect()
					s.logger.Infof("starting %d direct handle loops on each CPU thread", numCPU)
					for i := 0; i < numCPU; i++ {
						go s.handleLoopDirect()
					}
				} else {
					go s.channelHandler()
					go s.parsePortMappings()

					s.logger.Infof("starting %d handle loops on each CPU thread", numCPU)

					for i := 0; i < numCPU; i++ {
						go s.handleLoop()
					}
				}

				s.config.TunnelStatus = fmt.Sprintf("Connected (%s)", s.config.Mode)

			} else if strings.HasPrefix(r.URL.Path, "/tunnel") {
				wsConn := TunnelChannel{
					conn: conn,
					ping: make(chan struct{}),
					mu:   &sync.Mutex{},
				}
				select {
				case s.tunnelChannel <- wsConn:
					go s.keepAlive(&wsConn)
					s.logger.Debugf("websocket connection accepted from %s", conn.RemoteAddr().String())
				default:
					s.logger.Warnf("websocket tunnel channel is full, closing connection from %s", conn.RemoteAddr().String())
					conn.Close()
				}
			}
		}),
	}

	if s.config.Mode == config.WS {
		go func() {
			s.logger.Infof("ws server starting, listening on %s", addr)
			if s.controlChannel == nil {
				s.logger.Info("waiting for ws control channel connection")
			}
			if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				s.logger.Fatalf("failed to listen on %s: %v", addr, err)
			}
		}()
	} else {
		go func() {
			host, _, _ := net.SplitHostPort(addr)
			if host == "" {
				host = "localhost"
			}
			err := utils.EnsureSelfSignedCert(s.config.TLSCertFile, s.config.TLSKeyFile, host)
			if err != nil {
				s.logger.Fatalf("failed to generate self-signed certificate: %v", err)
			}
			s.logger.Infof("wss server starting, listening on %s", addr)
			if s.controlChannel == nil {
				s.logger.Info("waiting for wss control channel connection")
			}
			if err := server.ListenAndServeTLS(s.config.TLSCertFile, s.config.TLSKeyFile); err != nil && err != http.ErrServerClosed {
				s.logger.Fatalf("failed to listen on %s: %v", addr, err)
			}
		}()
	}

	<-s.ctx.Done()

	// Gracefully shutdown the server
	s.logger.Infof("shutting down the webSocket server on %s", addr)
	if err := server.Shutdown(context.Background()); err != nil {
		s.logger.Errorf("Failed to gracefully shutdown the server: %v", err)
	}

	if s.controlChannel != nil {
		s.controlChannel.Close()
	}

}

func (s *WsTransport) parsePortMappings() {
	mapping := utils.ParsePortsToListenerConfig(s.config.Ports)

	for laddr, cfg := range mapping {
		go s.localListener(laddr, cfg)
	}
}

func (s *WsTransport) localListener(localAddr string, cfg utils.ListenerConfig) {
	portListener, err := net.Listen("tcp", localAddr)
	if err != nil {
		if strings.Contains(err.Error(), "address already in use") {
			s.logger.Fatalf("failed to start listener on %s: %v", localAddr, err)
		} else {
			s.logger.Warnf("failed to start listener on %s (remote service may be unavailable): %v", localAddr, err)
		}
		return
	}

	//close local listener after context cancellation
	defer portListener.Close()

	s.logger.Infof("listener started successfully, listening on address: %s", portListener.Addr().String())

	go s.acceptLocalConn(portListener, cfg)

	<-s.ctx.Done()
}

func (s *WsTransport) acceptLocalConn(listener net.Listener, cfg utils.ListenerConfig) {
	for {
		select {
		case <-s.ctx.Done():
			return

		default:
			s.logger.Debugf("waiting to accept incoming connection on %s", listener.Addr().String())
			conn, err := listener.Accept()
			if err != nil {
				s.logger.Debugf("failed to accept connection on %s: %v", listener.Addr().String(), err)
				continue
			}

			// discard any non-tcp connection
			tcpConn, ok := conn.(*net.TCPConn)
			if !ok {
				s.logger.Warnf("disarded non-TCP connection from %s", conn.RemoteAddr().String())
				conn.Close()
				continue
			}

			// trying to enable tcpnodelay
			if !s.config.Nodelay {
				if err := tcpConn.SetNoDelay(s.config.Nodelay); err != nil {
					s.logger.Warnf("failed to set TCP_NODELAY for %s: %v", tcpConn.RemoteAddr().String(), err)
				} else {
					s.logger.Tracef("TCP_NODELAY disabled for %s", tcpConn.RemoteAddr().String())
				}
			}

			// Set keep-alive settings
			if err := tcpConn.SetKeepAlive(true); err != nil {
				s.logger.Warnf("failed to enable TCP keep-alive for %s: %v", tcpConn.RemoteAddr().String(), err)
			} else {
				s.logger.Tracef("TCP keep-alive enabled for %s", tcpConn.RemoteAddr().String())
			}
			if err := tcpConn.SetKeepAlivePeriod(s.config.KeepAlive); err != nil {
				s.logger.Warnf("failed to set TCP keep-alive period for %s: %v", tcpConn.RemoteAddr().String(), err)
			}

			// Attempt host-based routing first
			var selectedRemote string
			if len(cfg.HostMap) > 0 {
				peekRes, err := utils.PeekHostFromConn(conn, 100*time.Millisecond, 4096)
				if err == nil || err == utils.ErrNoData {
					if peekRes.Conn != nil {
						conn = peekRes.Conn
					}
					if peekRes.Host != "" {
						if target, ok := cfg.HostMap[peekRes.Host]; ok {
							selectedRemote = target
						}
					}
				} else {
					s.logger.Debugf("host peek failed: %v", err)
				}
			}

			if selectedRemote == "" {
				selectedRemote = utils.SelectBySrcDstHash(conn.RemoteAddr().String(), listener.Addr().String(), cfg.Remotes)
			}

			select {
			case s.localChannel <- LocalTCPConn{conn: conn, remoteAddr: selectedRemote, timeCreated: time.Now().UnixMilli()}:

				select {
				case s.reqNewConnChan <- struct{}{}:
					// Successfully requested a new connection
				default:
					// The channel is full, do nothing
					s.logger.Warn("channel is full, cannot request a new connection")
				}

				s.logger.Debugf("accepted incoming TCP connection from %s -> %s", tcpConn.RemoteAddr().String(), selectedRemote)

			default: // channel is full, discard the connection
				s.logger.Warnf("channel with listener %s is full, discarding TCP connection from %s", listener.Addr().String(), tcpConn.LocalAddr().String())
				conn.Close()
			}
		}
	}
}

func (s *WsTransport) channelHandlerDirect() {
	const maxRetries = 3
	const baseBackoff = time.Second
	ticker := time.NewTicker(s.config.Heartbeat)
	defer ticker.Stop()
	messageChan := make(chan byte, 10)

	go func() {
		retries := 0
		for {
			select {
			case <-s.ctx.Done():
				return
			default:
				_, msg, err := s.controlChannel.ReadMessage()
				if err != nil {
					s.logger.Errorf("failed to read from channel connection (try %d/%d): %v", retries+1, maxRetries, err)
					retries++
					if retries >= maxRetries {
						if s.cancel != nil {
							s.logger.Error("max retries reached, restarting...")
							go s.Restart()
						}
						return
					}
					time.Sleep(baseBackoff * time.Duration(retries))
					continue
				}
				retries = 0
				if len(msg) == 0 {
					s.logger.Warn("received empty message from control channel")
					continue
				}
				messageChan <- msg[0]
			}
		}
	}()

	rtt := time.Now()
	err := s.controlChannel.WriteMessage(websocket.BinaryMessage, []byte{utils.SG_RTT})
	if err != nil {
		s.logger.Error("failed to send RTT signal, attempting to restart server...")
		go s.Restart()
		return
	}

	for {
		select {
		case <-s.ctx.Done():
			_ = s.controlChannel.WriteMessage(websocket.BinaryMessage, []byte{utils.SG_Closed})
			return
		case <-ticker.C:
			err := s.controlChannel.WriteMessage(websocket.BinaryMessage, []byte{utils.SG_HB})
			if err != nil {
				s.logger.Errorf("failed to send heartbeat signal. Error: %v.", err)
				go s.Restart()
				return
			}
		case msg, ok := <-messageChan:
			if !ok {
				s.logger.Error("channel closed, likely due to an error in WebSocket read")
				return
			}
			switch msg {
			case utils.SG_HB:
				s.logger.Trace("heartbeat signal received successfully")
			case utils.SG_Closed:
				s.logger.Warn("control channel has been closed by the client")
				s.Restart()
				return
			case utils.SG_RTT:
				measureRTT := time.Since(rtt)
				s.logger.Infof("Round Trip Time (RTT): %d ms", measureRTT.Milliseconds())
			default:
				s.logger.Errorf("unexpected response from channel: %v", msg)
				go s.Restart()
				return
			}
		}
	}
}

func (s *WsTransport) handleLoopDirect() {
	for {
		select {
		case <-s.ctx.Done():
			return
		case tunnelConnection := <-s.tunnelChannel:
			go s.handleDirectConnection(tunnelConnection)
		}
	}
}

func (s *WsTransport) handleDirectConnection(tunnelConnection TunnelChannel) {
	close(tunnelConnection.ping)
	
	_, remoteAddrBytes, err := tunnelConnection.conn.ReadMessage()
	if err != nil {
		s.logger.Debugf("failed to read from direct tunnel: %v", err)
		tunnelConnection.conn.Close()
		return
	}
	if len(remoteAddrBytes) == 0 {
		tunnelConnection.conn.Close()
		return
	}
	if remoteAddrBytes[0] == utils.SG_Ping {
		// Ping
		tunnelConnection.conn.Close()
		return
	}

	remoteAddr, tunTransport, err := utils.DecodeTransportStringBytes(remoteAddrBytes)
	if err != nil {
		s.logger.Debugf("failed to decode transport string: %v", err)
		tunnelConnection.conn.Close()
		return
	}

	port, resolvedAddr, err := transport.ResolveRemoteAddr(remoteAddr)
	if err != nil {
		tunnelConnection.conn.Close()
		return
	}

	switch tunTransport {
	case utils.SG_TCP:
		localConnection, err := transport.TcpDialer(s.ctx, resolvedAddr, 10*time.Second, s.config.KeepAlive, true, 1, 32*1024, 32*1024, s.logger)
		if err != nil {
			tunnelConnection.conn.Close()
			return
		}
		utils.WSConnectionHandler(tunnelConnection.conn, localConnection, s.logger, s.usageMonitor, port, s.config.Sniffer)
	case utils.SG_UDP:
		// Not implemented for WS non-mux yet, but fallback just closes
		tunnelConnection.conn.Close()
	default:
		tunnelConnection.conn.Close()
	}
}

func (s *WsTransport) handleLoop() {
	for {
		select {
		case <-s.ctx.Done():
			return
		case localConn := <-s.localChannel:
		loop:
			for {
				if time.Now().UnixMilli()-localConn.timeCreated > 3000 { // 3000ms
					s.logger.Debugf("timeouted local connection: %d ms", time.Now().UnixMilli()-localConn.timeCreated)
					localConn.conn.Close()
					break loop
				}

				select {
				case <-s.ctx.Done():
					return
				case tunnelConnection := <-s.tunnelChannel:
					close(tunnelConnection.ping)
					tunnelConnection.mu.Lock()
					if err := tunnelConnection.conn.WriteMessage(websocket.TextMessage, []byte(localConn.remoteAddr)); err != nil {
						s.logger.Debugf("%v", err) // failed to send port number
						tunnelConnection.conn.Close()
						continue loop
					}
					// Handle data exchange between connections
					go utils.WSConnectionHandler(tunnelConnection.conn, localConn.conn, s.logger, s.usageMonitor, localConn.conn.LocalAddr().(*net.TCPAddr).Port, s.config.Sniffer)
					break loop
				}
			}
		}
	}
}

func (s *WsTransport) keepAlive(conn *TunnelChannel) {
	ticker := time.NewTicker(s.config.Heartbeat) // Send periodic pings to the client

	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			conn.conn.Close()
			return
		case <-conn.ping:
			s.logger.Trace("ping channel closed")
			return
		case <-ticker.C:
			// Try to acquire the lock without blocking
			locked := conn.mu.TryLock()
			if !locked {
				// If the lock is held by another operation, stop the pingSender
				s.logger.Trace("write operation in progress, stopping pingSender")
				return
			}

			if err := conn.conn.WriteMessage(websocket.BinaryMessage, []byte{utils.SG_Ping}); err != nil {
				conn.mu.Unlock()
				conn.conn.Close()
				return
			}
			conn.mu.Unlock()
			s.logger.Trace("ping sent to the client")
		}
	}
}
