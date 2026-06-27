package transport

import (
	"context"
	"net"
	"time"

	"github.com/musix/backhaul/internal/utils"
)

func (c *QuicTransport) parsePortMappings() {
	mapping := utils.ParsePortsToListenerConfig(c.config.Ports)
	for laddr, cfg := range mapping {
		go c.startListeners(laddr, cfg)
	}
}

func (c *QuicTransport) startListeners(localAddr string, cfg utils.ListenerConfig) {
	go c.localListener(localAddr, cfg)
	
	if c.config.AcceptUDP {
		// UDP not supported natively in this snippet
	}

	c.logger.Debugf("Started listening on %s, forwarding to %v (hosts=%v)", localAddr, cfg.Remotes, cfg.HostMap)
}

func (c *QuicTransport) localListener(localAddr string, cfg utils.ListenerConfig) {
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

func (c *QuicTransport) acceptLocalConn(listener net.Listener, cfg utils.ListenerConfig) {
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

func (c *QuicTransport) handleLoopDirect() {
	for {
		select {
		case <-c.ctx.Done():
			return
		case localConn := <-c.localChannel:
			c.logger.Debug("handleLoopDirect: processing a local connection")
			
			c.activeMu.Lock()
			c.activeConnections++
			c.activeMu.Unlock()

			if c.controlChannel == nil {
				c.logger.Warn("quic control channel is nil, cannot bridge. Restarting...")
				localConn.conn.Close()
				c.activeMu.Lock()
				c.activeConnections--
				c.activeMu.Unlock()
				go c.Restart()
				continue
			}

			go c.bridgeDirect(localConn)
		}
	}
}

func (c *QuicTransport) bridgeDirect(localConn LocalTCPConn) {
	defer func() {
		c.activeMu.Lock()
		c.activeConnections--
		c.activeMu.Unlock()
	}()

	stream, err := c.controlChannel.OpenStreamSync(context.Background())
	if err != nil {
		c.logger.Errorf("failed to open stream: %v", err)
		localConn.conn.Close()
		return
	}

	err = utils.SendBinaryTransportString(stream, localConn.remoteAddr, utils.SG_TCP)
	if err != nil {
		c.logger.Errorf("failed to send selected remote to tunnel: %v", err)
		stream.Close()
		localConn.conn.Close()
		return
	}

	c.logger.Debugf("connected to direct quic tunnel successfully, remote: %s", localConn.remoteAddr)
	
	port := 0
	if parsedPort, _, err := ResolveRemoteAddr(localConn.remoteAddr); err == nil {
		port = parsedPort
	}
	utils.QConnectionHandler(localConn.conn, stream, c.logger, c.usageMonitor, port, c.config.Sniffer)
}
