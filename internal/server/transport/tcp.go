package transport

import (
	"context"
	"fmt"
	"net"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/musix/backhaul/internal/client/transport"
	"github.com/musix/backhaul/internal/utils"
	"github.com/musix/backhaul/internal/web"

	"github.com/sirupsen/logrus"
)

type TcpTransport struct {
	config         *TcpConfig
	parentctx      context.Context
	ctx            context.Context
	cancel         context.CancelFunc
	logger         *logrus.Logger
	tunnelChannel  chan net.Conn
	localChannel   chan LocalTCPConn
	reqNewConnChan chan struct{}
	controlChannel net.Conn
	restartMutex   sync.Mutex
	usageMonitor   *web.Usage
	rtt            int64 // in ms, for UDP
}

type TcpConfig struct {
	BindAddr       string
	Token          string
	SnifferLog     string
	TunnelStatus   string
	Ports          []string
	TunnelMode     string
	Nodelay        bool
	Sniffer        bool
	KeepAlive      time.Duration
	Heartbeat      time.Duration // in seconds
	ChannelSize    int
	WebPort        int
	AcceptUDP      bool
	AllowedClients []string // Whitelist of allowed client IPs/domains
}

func NewTCPServer(parentCtx context.Context, config *TcpConfig, logger *logrus.Logger) *TcpTransport {
	// Create a derived context from the parent context
	ctx, cancel := context.WithCancel(parentCtx)

	// Initialize the TcpTransport struct
	server := &TcpTransport{
		config:         config,
		parentctx:      parentCtx,
		ctx:            ctx,
		cancel:         cancel,
		logger:         logger,
		tunnelChannel:  make(chan net.Conn, config.ChannelSize),
		localChannel:   make(chan LocalTCPConn, config.ChannelSize),
		reqNewConnChan: make(chan struct{}, config.ChannelSize),
		controlChannel: nil, // will be set when a control connection is established
		usageMonitor:   web.NewDataStore(fmt.Sprintf(":%v", config.WebPort), ctx, config.SnifferLog, config.Sniffer, &config.TunnelStatus, logger),
		rtt:            0,
	}

	return server
}

func (s *TcpTransport) Start() {
	s.config.TunnelStatus = "Disconnected (TCP)"

	if s.config.WebPort > 0 {
		go s.usageMonitor.Monitor()
	}

	go s.tunnelListener()

	s.channelHandshake()

	if s.controlChannel != nil {
		s.config.TunnelStatus = "Connected (TCP)"

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
			go s.parsePortMappings()
			go s.channelHandler()

			s.logger.Infof("starting %d handle loops on each CPU thread", numCPU)

			for i := 0; i < numCPU; i++ {
				go s.handleLoop()
			}
		}
	}
}
func (s *TcpTransport) Restart() {
	if !s.restartMutex.TryLock() {
		s.logger.Warn("server restart already in progress, skipping restart attempt")
		return
	}
	defer s.restartMutex.Unlock()

	s.logger.Info("restarting server...")

	// for removing timeout logs
	level := s.logger.Level
	s.logger.SetLevel(logrus.FatalLevel)

	if s.cancel != nil {
		s.cancel()
	}

	// Close open connection
	if s.controlChannel != nil {
		s.controlChannel.Close()
	}

	time.Sleep(2 * time.Second)

	ctx, cancel := context.WithCancel(s.parentctx)
	s.ctx = ctx
	s.cancel = cancel

	// Re-initialize variables
	s.tunnelChannel = make(chan net.Conn, s.config.ChannelSize)
	s.localChannel = make(chan LocalTCPConn, s.config.ChannelSize)
	s.reqNewConnChan = make(chan struct{}, s.config.ChannelSize)
	s.usageMonitor = web.NewDataStore(fmt.Sprintf(":%v", s.config.WebPort), ctx, s.config.SnifferLog, s.config.Sniffer, &s.config.TunnelStatus, s.logger)
	s.config.TunnelStatus = ""
	s.controlChannel = nil

	// set the log level again
	s.logger.SetLevel(level)

	go s.Start()
}

func (s *TcpTransport) channelHandshake() {
	for {
		select {
		case <-s.ctx.Done():
			return
		case conn := <-s.tunnelChannel:
			// Set a read deadline for the token response
			if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
				s.logger.Errorf("failed to set read deadline: %v", err)
				conn.Close()
				continue
			}

			msg, transport, err := utils.ReceiveBinaryTransportString(conn)
			if transport != utils.SG_Chan {
				s.logger.Errorf("invalid signal received for channel, Discarding connection")
				conn.Close()
				continue
			} else if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					s.logger.Warn("timeout while waiting for control channel signal")
				} else {
					s.logger.Errorf("failed to receive control channel signal: %v", err)
				}
				conn.Close() // Close connection on error or timeout
				continue
			}

			// Resetting the deadline (removes any existing deadline)
			conn.SetReadDeadline(time.Time{})

			if msg != s.config.Token {
				s.logger.Warnf("invalid security token received: %s", msg)
				conn.Close()
				continue
			}

			err = utils.SendBinaryTransportString(conn, s.config.Token, utils.SG_Chan)
			if err != nil {
				s.logger.Errorf("failed to send security token: %v", err)
				conn.Close()
				continue
			}

			s.controlChannel = conn

			s.logger.Info("control channel successfully established.")
			return
		}
	}
}

func (s *TcpTransport) channelHandler() {
	const maxRetries = 3
	const baseBackoff = time.Second
	ticker := time.NewTicker(s.config.Heartbeat)
	defer ticker.Stop()

	// Channel to receive the message or error
	messageChan := make(chan byte, 1)

	go func() {
		retries := 0
		for {
			select {
			case <-s.ctx.Done():
				return
			default:
				message, err := utils.ReceiveBinaryByte(s.controlChannel)
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
				messageChan <- message
			}
		}
	}()

	// RTT measurment
	rtt := time.Now()
	err := utils.SendBinaryByte(s.controlChannel, utils.SG_RTT)
	if err != nil {
		s.logger.Error("failed to send RTT signal, attempting to restart server...")
		go s.Restart()
		return
	}

	for {
		select {
		case <-s.ctx.Done():
			_ = utils.SendBinaryByte(s.controlChannel, utils.SG_Closed)
			return

		case <-s.reqNewConnChan:
			err := utils.SendBinaryByte(s.controlChannel, utils.SG_Chan)
			if err != nil {
				s.logger.Error("failed to send request new connection signal. ", err)
				go s.Restart()
				return
			}

		case <-ticker.C:
			err := utils.SendBinaryByte(s.controlChannel, utils.SG_HB)
			if err != nil {
				s.logger.Error("failed to send heartbeat signal")
				go s.Restart()
				return
			}
			s.logger.Trace("heartbeat signal sent successfully")

		case message, ok := <-messageChan:
			if !ok {
				s.logger.Error("channel closed, likely due to an error in TCP read")
				return
			}

			switch message {
			case utils.SG_Closed:
				s.logger.Warn("control channel has been closed by the client")
				go s.Restart()
				return
			case utils.SG_RTT:
				measureRTT := time.Since(rtt)
				s.rtt = measureRTT.Milliseconds()
				s.logger.Infof("Round Trip Time (RTT): %d ms", s.rtt)
			}
		}
	}
}

func (s *TcpTransport) tunnelListener() {
	listener, err := net.Listen("tcp", s.config.BindAddr)
	if err != nil {
		s.logger.Fatalf("failed to start listener on %s: %v", s.config.BindAddr, err)
		return
	}

	defer listener.Close()

	s.logger.Infof("server started successfully, listening on address: %s", listener.Addr().String())

	go s.acceptTunnelConn(listener)

	<-s.ctx.Done()
}

func (s *TcpTransport) acceptTunnelConn(listener net.Listener) {
	for {
		select {
		case <-s.ctx.Done():
			return
		default:
			s.logger.Debugf("waiting for accept incoming tunnel connection on %s", listener.Addr().String())
			conn, err := listener.Accept()
			if err != nil {
				s.logger.Debugf("failed to accept tunnel connection on %s: %v", listener.Addr().String(), err)
				continue
			}

			//discard any non tcp connection
			tcpConn, ok := conn.(*net.TCPConn)
			if !ok {
				s.logger.Warnf("disarded non-TCP tunnel connection from %s", conn.RemoteAddr().String())
				conn.Close()
				continue
			}

			// Check if client is in whitelist
			if !utils.IsClientAllowed(tcpConn.RemoteAddr().String(), s.config.AllowedClients) {
				s.logger.Warnf("client %s is not in whitelist, rejecting connection", tcpConn.RemoteAddr().String())
				tcpConn.Close()
				continue
			}

			// Drop all suspicious packets from other address rather than server
			if s.controlChannel != nil && s.controlChannel.RemoteAddr().(*net.TCPAddr).IP.String() != tcpConn.RemoteAddr().(*net.TCPAddr).IP.String() {
				s.logger.Debugf("suspicious packet from %v. expected address: %v. discarding packet...", tcpConn.RemoteAddr().(*net.TCPAddr).IP.String(), s.controlChannel.RemoteAddr().(*net.TCPAddr).IP.String())
				tcpConn.Close()
				continue
			}

			// trying to set tcpnodelay
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

			select {
			case s.tunnelChannel <- conn:
			default: // The channel is full, do nothing
				s.logger.Debugf("tunnel listener channel is full, discarding TCP connection from %s", conn.LocalAddr().String())
				conn.Close()
			}
		}
	}
}

func (s *TcpTransport) parsePortMappings() {
	// Use the shared parser that understands host:port left-hand side values
	mapping := utils.ParsePortsToListenerConfig(s.config.Ports)

	for laddr, cfg := range mapping {
		go s.startListeners(laddr, cfg)
	}
}

func (s *TcpTransport) startListeners(localAddr string, cfg utils.ListenerConfig) {
	// Start TCP listener
	go s.localListener(localAddr, cfg)

	// Start UDP listener if configured
	if s.config.AcceptUDP {
		go s.udpListener(localAddr, cfg.Remotes)
	}

	s.logger.Debugf("Started listening on %s, forwarding to %v (hosts=%v)", localAddr, cfg.Remotes, cfg.HostMap)
}

func (s *TcpTransport) localListener(localAddr string, cfg utils.ListenerConfig) {
	listener, err := net.Listen("tcp", localAddr)
	if err != nil {
		if strings.Contains(err.Error(), "address already in use") {
			s.logger.Fatalf("failed to listen on %s: %v", localAddr, err)
		} else {
			s.logger.Warnf("failed to listen on %s (remote service may be unavailable): %v", localAddr, err)
		}
		return
	}

	defer listener.Close()

	s.logger.Infof("listener started successfully, listening on address: %s", listener.Addr().String())

	go s.acceptLocalConn(listener, cfg)

	<-s.ctx.Done()
}

func (s *TcpTransport) acceptLocalConn(listener net.Listener, cfg utils.ListenerConfig) {
	for {
		select {
		case <-s.ctx.Done():
			return

		default:
			s.logger.Debugf("waiting for accept incoming connection on %s", listener.Addr().String())
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

			// trying to disable tcpnodelay
			if !s.config.Nodelay {
				if err := tcpConn.SetNoDelay(s.config.Nodelay); err != nil {
					s.logger.Warnf("failed to set TCP_NODELAY for %s: %v", tcpConn.RemoteAddr().String(), err)
				} else {
					s.logger.Tracef("TCP_NODELAY disabled for %s", tcpConn.RemoteAddr().String())
				}
			}

			// Determine target remote based on Host/SNI when available
			var selectedRemote string
			if len(cfg.HostMap) > 0 {
				// peek into the connection for HTTP Host header or TLS SNI
				peekRes, err := utils.PeekHostFromConn(conn, 100*time.Millisecond, 4096)
				if err == nil || err == utils.ErrNoData {
					// use wrapped conn if provided so buffered bytes are preserved
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
				// fallback to existing selection/load-balance behaviour
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

func (s *TcpTransport) handleLoop() {
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

				case tunnelConn := <-s.tunnelChannel:
					// Send the target addr over the connection
					if err := utils.SendBinaryTransportString(tunnelConn, localConn.remoteAddr, utils.SG_TCP); err != nil {
						s.logger.Errorf("%v", err)
						tunnelConn.Close()
						continue loop
					}

					// Handle data exchange between connections
					go utils.TCPConnectionHandler(localConn.conn, tunnelConn, s.logger, s.usageMonitor, localConn.conn.LocalAddr().(*net.TCPAddr).Port, s.config.Sniffer)
					break loop

				}
			}
		}
	}
}

func (s *TcpTransport) channelHandlerDirect() {
	const maxRetries = 3
	const baseBackoff = time.Second
	ticker := time.NewTicker(s.config.Heartbeat)
	defer ticker.Stop()

	messageChan := make(chan byte, 1)

	go func() {
		retries := 0
		for {
			select {
			case <-s.ctx.Done():
				return
			default:
				message, err := utils.ReceiveBinaryByte(s.controlChannel)
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
				messageChan <- message
			}
		}
	}()

	rtt := time.Now()
	err := utils.SendBinaryByte(s.controlChannel, utils.SG_RTT)
	if err != nil {
		s.logger.Error("failed to send RTT signal, attempting to restart server...")
		go s.Restart()
		return
	}

	for {
		select {
		case <-s.ctx.Done():
			_ = utils.SendBinaryByte(s.controlChannel, utils.SG_Closed)
			return

		case <-ticker.C:
			err := utils.SendBinaryByte(s.controlChannel, utils.SG_HB)
			if err != nil {
				s.logger.Error("failed to send heartbeat signal")
				go s.Restart()
				return
			}

		case message, ok := <-messageChan:
			if !ok {
				s.logger.Error("channel closed, likely due to an error in TCP read")
				return
			}

			switch message {
			case utils.SG_Closed:
				s.logger.Warn("control channel has been closed by the client")
				go s.Restart()
				return
			case utils.SG_RTT:
				measureRTT := time.Since(rtt)
				s.rtt = measureRTT.Milliseconds()
				s.logger.Infof("Round Trip Time (RTT): %d ms", s.rtt)
			}
		}
	}
}

func (s *TcpTransport) handleLoopDirect() {
	for {
		select {
		case <-s.ctx.Done():
			return
		case tunnelConn := <-s.tunnelChannel:
			go func(tConn net.Conn) {
				remoteAddr, tunTransport, err := utils.ReceiveBinaryTransportString(tConn)
				if err != nil {
					s.logger.Debugf("failed to receive remote addr from tunnel: %v", err)
					tConn.Close()
					return
				}

				port, resolvedAddr, err := transport.ResolveRemoteAddr(remoteAddr)
				if err != nil {
					s.logger.Infof("failed to resolve remote port: %v", err)
					tConn.Close()
					return
				}

				switch tunTransport {
				case utils.SG_TCP:
					localConnection, err := transport.TcpDialer(s.ctx, resolvedAddr, 10*time.Second, s.config.KeepAlive, true, 1, 32*1024, 32*1024, s.logger)
					if err != nil {
						if strings.Contains(err.Error(), "connection refused") || strings.Contains(err.Error(), "Connection refused") {
							s.logger.Tracef("local dialer: service unavailable at %s: %v", resolvedAddr, err)
						} else {
							s.logger.Warnf("local dialer: failed to connect to %s: %v", resolvedAddr, err)
						}
						tConn.Close()
						return
					}
					s.logger.Debugf("connected to local address %s successfully", resolvedAddr)
					utils.TCPConnectionHandler(tConn, localConnection, s.logger, s.usageMonitor, port, s.config.Sniffer)
				case utils.SG_UDP:
					transport.UDPDialer(tConn, resolvedAddr, s.logger, s.usageMonitor, port, s.config.Sniffer)
				default:
					s.logger.Error("undefined transport. close the connection.")
					tConn.Close()
				}
			}(tunnelConn)
		}
	}
}
