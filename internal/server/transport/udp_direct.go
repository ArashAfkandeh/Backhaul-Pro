package transport

import (
	"net"
	"time"

	"github.com/musix/backhaul/internal/utils"
	"github.com/musix/backhaul/internal/client/transport"
)

func (s *UdpTransport) handleDirectTunnelUDPConn(tunnelConn *TunnelUDPConn) {
	// First packet in payload should be the target address
	select {
	case <-s.ctx.Done():
		return
	case data, ok := <-tunnelConn.payload:
		if !ok {
			return
		}
		
		remoteAddr, tunTransport, err := utils.DecodeTransportStringBytes(data)
		if err != nil {
			s.logger.Errorf("failed to decode target address from direct UDP tunnel: %v", err)
			return
		}

		port, resolvedAddr, err := transport.ResolveRemoteAddr(remoteAddr)
		if err != nil {
			s.logger.Errorf("failed to resolve target address: %v", err)
			return
		}

		switch tunTransport {
		case utils.SG_UDP:
			remoteUDPAddr, err := net.ResolveUDPAddr("udp", resolvedAddr)
			if err != nil {
				return
			}
			remoteConn, err := net.DialUDP("udp", nil, remoteUDPAddr)
			if err != nil {
				return
			}
			defer remoteConn.Close()

			done := make(chan struct{})
			go func() {
				defer close(done)
				for {
					select {
					case d, ok := <-tunnelConn.payload:
						if !ok {
							return
						}
						remoteConn.Write(d)
					case <-time.After(60 * time.Second):
						return
					}
				}
			}()

			go func() {
				buf := make([]byte, 16*1024)
				for {
					remoteConn.SetReadDeadline(time.Now().Add(60 * time.Second))
					n, err := remoteConn.Read(buf)
					if err != nil {
						return
					}
					tunnelConn.listener.WriteToUDP(buf[:n], tunnelConn.addr)
					if s.config.Sniffer {
						s.usageMonitor.AddOrUpdatePort(port, uint64(n))
					}
				}
			}()

			<-done
		case utils.SG_TCP:
			// Not implemented for UDP bridging to TCP
		}
	case <-time.After(5 * time.Second):
		s.logger.Warn("timeout waiting for target address in UDP direct mode")
	}

	s.activeMu.Lock()
	close(tunnelConn.payload)
	delete(s.activeConnections, tunnelConn.addr.String())
	s.activeMu.Unlock()
}
