package transport

import (
	"net"
	"time"
	"sync/atomic"

	"github.com/musix/backhaul/internal/utils"
)

func (c *TcpTransport) parsePortMappings() {
	mapping := utils.ParsePortsToListenerConfig(c.config.Ports)
	for laddr, cfg := range mapping {
		go c.startListeners(laddr, cfg)
	}
}

func (c *TcpTransport) startListeners(localAddr string, cfg utils.ListenerConfig) {
	go c.localListener(localAddr, cfg)
	
	if c.config.AcceptUDP {
		// go c.udpListener(localAddr, cfg.Remotes)
	}

	c.logger.Debugf("Started listening on %s, forwarding to %v (hosts=%v)", localAddr, cfg.Remotes, cfg.HostMap)
}

func (c *TcpTransport) localListener(localAddr string, cfg utils.ListenerConfig) {
	listener, err := net.Listen("tcp", localAddr)
	if err != nil {
		c.logger.Warnf("failed to listen on %s: %v", localAddr, err)
		return
	}
	defer listener.Close()

	c.logger.Infof("client listener started successfully on: %s", listener.Addr().String())
	go c.acceptLocalConn(listener, cfg)
	<-c.ctx.Done()
}

func (c *TcpTransport) acceptLocalConn(listener net.Listener, cfg utils.ListenerConfig) {
	for {
		select {
		case <-c.ctx.Done():
			return
		default:
			conn, err := listener.Accept()
			if err != nil {
				continue
			}
			tcpConn, ok := conn.(*net.TCPConn)
			if !ok {
				conn.Close()
				continue
			}
			if !c.config.Nodelay {
				tcpConn.SetNoDelay(c.config.Nodelay)
			}
			
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
				}
			}

			if selectedRemote == "" {
				selectedRemote = utils.SelectBySrcDstHash(conn.RemoteAddr().String(), listener.Addr().String(), cfg.Remotes)
			}

			select {
			case c.localChannel <- LocalTCPConn{conn: conn, remoteAddr: selectedRemote, timeCreated: time.Now().UnixMilli()}:
			default:
				c.logger.Warnf("local listener channel is full")
				conn.Close()
			}
		}
	}
}

func (c *TcpTransport) handleLoopDirect() {
	for {
		select {
		case <-c.ctx.Done():
			return
		case localConn := <-c.localChannel:
			c.logger.Debug("handleLoopDirect: processing a local connection")
			
			atomic.AddInt32(&c.loadConnections, 1)

			select {
			case <-c.controlFlow: // Do nothing
			default:
				go c.tunnelDialer()
			}

			// Get pre-established tunnel connection from pool
			select {
			case tunnelConn := <-c.tunnelChannel:
				atomic.AddInt32(&c.poolConnections, -1) // Decrement since we are using it
				go c.bridgeDirect(localConn, tunnelConn)
			default:
				// Fallback if pool is empty
				go c.tunnelDialerDirect(localConn)
			}
		}
	}
}

func (c *TcpTransport) bridgeDirect(localConn LocalTCPConn, tunnelConn net.Conn) {
	err := utils.SendBinaryTransportString(tunnelConn, localConn.remoteAddr, utils.SG_TCP)
	if err != nil {
		c.logger.Errorf("failed to send selected remote to tunnel: %v", err)
		tunnelConn.Close()
		localConn.conn.Close()
		return
	}

	c.logger.Debugf("connected to direct tunnel successfully, remote: %s", localConn.remoteAddr)
	
	// Default port for usage monitor logic, it extracts the target port
	port := 0
	if parsedPort, _, err := ResolveRemoteAddr(localConn.remoteAddr); err == nil {
		port = parsedPort
	}
	utils.TCPConnectionHandler(localConn.conn, tunnelConn, c.logger, c.usageMonitor, port, c.config.Sniffer)
}

func (c *TcpTransport) tunnelDialerDirect(localConn LocalTCPConn) {
	c.logger.Debugf("initiating new direct tunnel to %s", c.config.RemoteAddr)

	tcpConn, err := TcpDialer(c.ctx, c.config.RemoteAddr, c.config.DialTimeOut, c.config.KeepAlive, c.config.Nodelay, 3, 1024*1024, 1024*1024, c.logger)
	if err != nil {
		c.logger.Error("direct tunnel server dialer: ", err)
		localConn.conn.Close()
		return
	}

	err = utils.SendBinaryTransportString(tcpConn, localConn.remoteAddr, utils.SG_TCP)
	if err != nil {
		c.logger.Errorf("failed to send selected remote to tunnel: %v", err)
		tcpConn.Close()
		localConn.conn.Close()
		return
	}

	c.logger.Debugf("connected to direct tunnel successfully, remote: %s", localConn.remoteAddr)
	
	// Default port for usage monitor logic, it extracts the target port
	port := 0
	if parsedPort, _, err := ResolveRemoteAddr(localConn.remoteAddr); err == nil {
		port = parsedPort
	}
	utils.TCPConnectionHandler(localConn.conn, tcpConn, c.logger, c.usageMonitor, port, c.config.Sniffer)
}
