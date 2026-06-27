package transport

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/musix/backhaul/internal/utils"
	"github.com/musix/backhaul/internal/web"
	"github.com/sirupsen/logrus"
)

type UdpTransport struct {
	config            *UdpConfig
	parentctx         context.Context
	ctx               context.Context
	cancel            context.CancelFunc
	logger            *logrus.Logger
	tunnelChannel     chan *TunnelUDPConn
	activeConnections map[string]*TunnelUDPConn
	activeMu          sync.Mutex
	reqNewConnChan    chan struct{}
	controlChannel    net.Conn
	restartMutex      sync.Mutex
	usageMonitor      *web.Usage
	rtt               int64 // for Fun!
}

type UdpConfig struct {
	BindAddr       string
	Token          string
	SnifferLog     string
	TunnelStatus   string
	Ports          []string
	Sniffer        bool
	Heartbeat      time.Duration // in seconds, for udp conn and control channel
	ChannelSize    int
	WebPort        int
	AllowedClients []string // Whitelist of allowed client IPs/domains
}

func NewUDPServer(parentCtx context.Context, config *UdpConfig, logger *logrus.Logger) *UdpTransport {
	// Create a derived context from the parent context
	ctx, cancel := context.WithCancel(parentCtx)

	// Initialize the TcpTransport struct
	server := &UdpTransport{
		config:            config,
		parentctx:         parentCtx,
		ctx:               ctx,
		cancel:            cancel,
		logger:            logger,
		tunnelChannel:     make(chan *TunnelUDPConn, config.ChannelSize),
		activeConnections: map[string]*TunnelUDPConn{},
		activeMu:          sync.Mutex{},
		reqNewConnChan:    make(chan struct{}, config.ChannelSize),
		controlChannel:    nil, // will be set when a control connection is established
		usageMonitor:      web.NewDataStore(fmt.Sprintf(":%v", config.WebPort), ctx, config.SnifferLog, config.Sniffer, &config.TunnelStatus, logger),
		rtt:               0,
	}

	return server
}
func (s *UdpTransport) Start() {
	s.config.TunnelStatus = "Disconnected (UDP)"

	if s.config.WebPort > 0 {
		go s.usageMonitor.Monitor()
	}

	go s.channelHandshake()
}

func (s *UdpTransport) Restart() {
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
	s.tunnelChannel = make(chan *TunnelUDPConn, s.config.ChannelSize)
	s.reqNewConnChan = make(chan struct{}, s.config.ChannelSize)
	s.usageMonitor = web.NewDataStore(fmt.Sprintf(":%v", s.config.WebPort), ctx, s.config.SnifferLog, s.config.Sniffer, &s.config.TunnelStatus, s.logger)
	s.config.TunnelStatus = ""
	s.controlChannel = nil
	s.activeConnections = map[string]*TunnelUDPConn{}
	s.activeMu = sync.Mutex{}

	// set the log level again
	s.logger.SetLevel(level)

	go s.Start()
}

func (s *UdpTransport) channelHandshake() {
	listener, err := net.Listen("tcp", s.config.BindAddr)
	if err != nil {
		s.logger.Fatalf("failed to start listener on %s: %v", s.config.BindAddr, err)
		return
	}

	s.logger.Infof("server started successfully, listening on address: %s", listener.Addr().String())

	defer listener.Close()

loop:
	for {
		select {
		case <-s.ctx.Done():
			return
		default:
			conn, err := listener.Accept()
			if err != nil {
				s.logger.Debugf("failed to accept control channel connection on %s: %v", listener.Addr().String(), err)
				continue
			}

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

			break loop
		}
	}

	go s.tunnelListener()
	go s.parsePortMappings()
	go s.channelHandler()

	<-s.ctx.Done()
}

func (s *UdpTransport) channelHandler() {
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

func (s *UdpTransport) tunnelListener() {
	tunnelUDPAddr, err := net.ResolveUDPAddr("udp", s.config.BindAddr)
	if err != nil {
		s.logger.Fatalf("failed to resolve tunnel address: %v", err)
	}

	listener, err := net.ListenUDP("udp", tunnelUDPAddr)
	if err != nil {
		s.logger.Fatalf("failed to listen on tunnel UDP port: %v", err)
	}

	defer listener.Close()

	s.logger.Infof("UDP tunnel listener started successfully, listening on address: %s", listener.LocalAddr().String())

	go s.acceptTunnelConn(listener)

	<-s.ctx.Done()
}

func (s *UdpTransport) acceptTunnelConn(listener *net.UDPConn) {
	// Buffer for UDP reads
	buf := make([]byte, 16*1024)

	for {
		select {
		case <-s.ctx.Done():
			return
		default:
			n, addr, err := listener.ReadFromUDP(buf)
			if err != nil {
				s.logger.Errorf("failed to read from tunnel UDP listener: %v", err)
				continue
			}

			// Create a unique identifier for the connection based on IP and port
			key := addr.String()

			// Check if client is in whitelist
			if !utils.IsClientAllowed(addr.String(), s.config.AllowedClients) {
				s.logger.Warnf("client %s is not in whitelist, rejecting UDP connection", addr.String())
				continue
			}

			s.activeMu.Lock()
			// Check if the connection is already active
			if existingConn, exists := s.activeConnections[key]; exists {
				// Send the payload to the existing connection's payload channel
				select {
				case existingConn.payload <- append([]byte(nil), buf[:n]...): // Copy the packet to avoid data overwriting
					s.logger.Tracef("buffered %d bytes for existing connection %s", n, addr.String())

				default:
					s.logger.Warnf("payload channel for connection %s is full, dropping UDP packet", addr.String())
				}
				s.activeMu.Unlock()
				continue
			}

			s.activeMu.Unlock()

			if string(buf[:n]) != s.config.Token { // For new connections, validate the token
				s.logger.Errorf("invalid token received from %s", addr.String())
				continue
			}

			// Initialize the payload channel for the new connection
			payloadChan := make(chan []byte, 100_000)

			// Create a new TunnelUDPConn
			tunnelConn := TunnelUDPConn{
				timeCreated: time.Now().UnixNano(), // Just for debugging
				payload:     payloadChan,
				addr:        addr,
				listener:    listener,
				ping:        make(chan struct{}, 1), // Initialize the ping channel
				mu:          &sync.Mutex{},
			}

			s.activeMu.Lock()
			// Add the new connection to the active connections map
			s.activeConnections[key] = &tunnelConn
			s.activeMu.Unlock()

			// Send the new tunnel connection to the tunnel channel
			select {
			case s.tunnelChannel <- &tunnelConn:
				go s.keepAlive(&tunnelConn)
				s.logger.Debugf("accepted tunnel connection from %s", addr.String())
			default:
				s.logger.Warn("UDP tunnel channel is full")
				// Close the newly created connection as it couldn't be added
				close(tunnelConn.payload)
				delete(s.activeConnections, key)
			}
		}
	}
}

func (s *UdpTransport) parsePortMappings() {
	mapping := utils.ParsePortsToListenerConfig(s.config.Ports)

	for laddr, cfg := range mapping {
		go s.localListener(laddr, cfg.Remotes)
	}
}

func (s *UdpTransport) localListener(localAddr string, remoteAddrs []string) {
	localUDPAddr, err := net.ResolveUDPAddr("udp", localAddr)
	if err != nil {
		s.logger.Warnf("failed to resolve local address: %v", err)
		return
	}

	listener, err := net.ListenUDP("udp", localUDPAddr)
	if err != nil {
		if strings.Contains(err.Error(), "address already in use") {
			s.logger.Fatalf("failed to listen on local UDP port: %v", err)
		} else {
			s.logger.Warnf("failed to listen on local UDP port (remote service may be unavailable): %v", err)
		}
		return
	}

	defer listener.Close()

	s.logger.Infof("UDP listener started successfully, listening on address: %s", listener.LocalAddr().String())

	// Buffer for UDP reads
	buf := make([]byte, 16*1024)

	// Track active connections
	activeConnections := map[string]*LocalUDPConn{}

	// mutex
	mu := &sync.Mutex{}

	// make a new channel for recieve udp packets
	udpChan := make(chan *LocalUDPConn, s.config.ChannelSize)

	// handle channel
	go s.handleLoop(udpChan, &activeConnections, mu)

	go func() {
		for {
			select {
			case <-s.ctx.Done():
				return
			default:
				n, addr, err := listener.ReadFromUDP(buf)
				if err != nil {
					s.logger.Errorf("failed to read from UDP listener: %v", err)
					continue
				}

				// Create a unique identifier for the connection based on IP and port
				key := addr.String()

				mu.Lock()
				// Check if the connection is already active
				if existingConn, exists := activeConnections[key]; exists {
					// If connection is active and not closed, send payload
					select {
					case existingConn.payload <- append([]byte(nil), buf[:n]...):
						s.logger.Tracef("buffered %d bytes for existing connection %s", n, addr.String())
					default:
						s.logger.Warnf("payload channel for connection %s is full, dropping UDP packet", addr.String())
					}
					mu.Unlock()
					continue
				}

				mu.Unlock()

				// Create a new payload channel for this connection, Buffer up to 100,000 packets for the connection
				payloadChan := make(chan []byte, 100_000)

				// Build the UDP connection object
				// select a remote address using source+destination hash
				selected := remoteAddrs[0]
				if len(remoteAddrs) > 1 {
					srcIP, _, _ := net.SplitHostPort(addr.String())
					dstIP, _, _ := net.SplitHostPort(listener.LocalAddr().String())
					selected = utils.SelectBySrcDstHash(srcIP, dstIP, remoteAddrs)
				}
				newUDPConn := LocalUDPConn{
					timeCreated: time.Now().UnixNano(), // Just for debugging
					payload:     payloadChan,
					remoteAddr:  selected,
					listener:    listener,
					addr:        addr,
				}

				mu.Lock()
				// Store the new connection
				activeConnections[key] = &newUDPConn
				mu.Unlock()

				select {
				case udpChan <- &newUDPConn:
					s.logger.Debugf("accepted UDP connection from %s", addr.String())
					payloadChan <- append([]byte(nil), buf[:n]...) // Send a copy of the new payload to the channel

					// Request a new TCP connection
					select {
					case s.reqNewConnChan <- struct{}{}:
						// Successfully requested a new TCP connection
					default:
						// The channel is full, do nothing
						s.logger.Warn("channel is full, cannot request a new connection")
					}

				default:
					s.logger.Warn("UDP channel is full, dropping packet.")
					// Close the newly created connection as it couldn't be added
					close(newUDPConn.payload)
					delete(activeConnections, key)
				}
			}
		}
	}()

	<-s.ctx.Done()

}

func (s *UdpTransport) handleLoop(udpChan chan *LocalUDPConn, activeConnections *map[string]*LocalUDPConn, mu *sync.Mutex) {
	for {
		select {
		case <-s.ctx.Done():
			return
		case localConn := <-udpChan:
			if time.Now().UnixMilli()-localConn.timeCreated > 3000 { // 3000ms
				s.logger.Debugf("timeouted local connection: %d ms", time.Now().UnixMilli()-localConn.timeCreated)
				continue
			}

		loop:
			for {
				select {
				case <-s.ctx.Done():
					return

				case tunnelConn := <-s.tunnelChannel:
					close(tunnelConn.ping)
					tunnelConn.mu.Lock()

					// Send the target addr over the connection
					if _, err := tunnelConn.listener.WriteTo([]byte(localConn.remoteAddr), tunnelConn.addr); err != nil {
						s.logger.Errorf("%v", err)
						continue loop
					}

					// Handle data exchange between connections
					go s.udpCopy(localConn, tunnelConn, activeConnections, mu)

					s.logger.Debugf("initiate new handler for connection %s with timestamp %d", localConn.addr.String(), localConn.timeCreated)
					break loop
				}
			}
		}
	}
}

func (s *UdpTransport) udpCopy(udpLocal *LocalUDPConn, udpTunnel *TunnelUDPConn, activeConnections *map[string]*LocalUDPConn, mu *sync.Mutex) {
	done := make(chan struct{})

	// Handle data from local to tunnel
	go func() {
		defer close(done)
		s.udpLocalCopy(udpLocal, udpTunnel)
	}()

	// Handle data from tunnel to local
	s.udpTunnelCopy(udpTunnel, udpLocal)

	// Wait until one of the directions is done (connection closed or idle)
	<-done

	// Remove local connection from active connections and close the channel
	mu.Lock()
	close(udpLocal.payload)
	delete(*activeConnections, udpLocal.addr.String())
	mu.Unlock()

	// Remove tunnel connection from active connections and close the channel
	s.activeMu.Lock()
	close(udpTunnel.payload)
	delete(s.activeConnections, udpTunnel.addr.String())
	s.activeMu.Unlock()

}

func (s *UdpTransport) udpLocalCopy(from *LocalUDPConn, to *TunnelUDPConn) {
	inactivityTimeout := 60 * time.Second // Define a 60-second inactivity timeout

	for {
		select {
		case data, ok := <-from.payload: // Wait for data on the UDP payload channel
			if !ok {
				return
			}

			packetSize := len(data)

			totalWritten := 0
			for totalWritten < packetSize {
				// Write the packet to the tunnel
				w, err := to.listener.WriteToUDP(data[totalWritten:], to.addr)
				if err != nil {
					s.logger.Errorf("failed to write UDP payload to tunnel: %v", err)
					return
				}
				totalWritten += w
			}

			if s.config.Sniffer {
				s.usageMonitor.AddOrUpdatePort(from.listener.LocalAddr().(*net.UDPAddr).Port, uint64(totalWritten))
			}

			s.logger.Debugf("forwarded %d bytes from local connection %s to tunnel", packetSize, from.addr.String())

		case <-time.After(inactivityTimeout): // Timeout after 30 seconds of inactivity
			s.logger.Debugf("connection idle for 60 seconds, closing UDP connection for %s", from.addr.String())
			return
		}
	}
}

func (s *UdpTransport) udpTunnelCopy(from *TunnelUDPConn, to *LocalUDPConn) {
	inactivityTimeout := 60 * time.Second // Define a 60-second inactivity timeout

	for {
		select {
		case data, ok := <-from.payload: // Wait for data on the UDP payload channel
			if !ok {
				return
			}

			packetSize := len(data)

			totalWritten := 0
			for totalWritten < packetSize {
				// Write the packet to the tunnel
				w, err := to.listener.WriteToUDP(data[totalWritten:], to.addr)
				if err != nil {
					s.logger.Errorf("failed to write UDP payload to tunnel: %v", err)
					return
				}
				totalWritten += w
			}

			if s.config.Sniffer {
				s.usageMonitor.AddOrUpdatePort(to.listener.LocalAddr().(*net.UDPAddr).Port, uint64(totalWritten))
			}

			s.logger.Debugf("forwarded %d bytes from local connection %s to tunnel", packetSize, from.addr.String())

		case <-time.After(inactivityTimeout): // Timeout after 30 seconds of inactivity
			s.logger.Debugf("connection idle for 60 seconds, closing UDP connection for %s", from.addr.String())
			return
		}
	}
}

func (s *UdpTransport) keepAlive(conn *TunnelUDPConn) {
	ticker := time.NewTicker(s.config.Heartbeat) // Send periodic pings to the client

	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
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
			if _, err := conn.listener.WriteTo([]byte{utils.SG_Ping}, conn.addr); err != nil {
				conn.mu.Unlock()
				return
			}
			conn.mu.Unlock()
			s.logger.Trace("ping sent to the client")
		}
	}
}
