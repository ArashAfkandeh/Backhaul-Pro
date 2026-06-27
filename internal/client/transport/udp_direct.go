package transport

import (
	"net"
	"sync"
	"time"

	"github.com/musix/backhaul/internal/utils"
)

type LocalUDPConn struct {
	timeCreated int64
	payload     chan []byte
	remoteAddr  string
	listener    *net.UDPConn
	addr        *net.UDPAddr
}

func (c *UdpTransport) parsePortMappingsDirect() {
	mapping := utils.ParsePortsToListenerConfig(c.config.Ports)
	for laddr, cfg := range mapping {
		go c.localListenerDirect(laddr, cfg.Remotes)
	}
}

func (c *UdpTransport) localListenerDirect(localAddr string, remoteAddrs []string) {
	localUDPAddr, err := net.ResolveUDPAddr("udp", localAddr)
	if err != nil {
		c.logger.Warnf("failed to resolve local address: %v", err)
		return
	}

	listener, err := net.ListenUDP("udp", localUDPAddr)
	if err != nil {
		c.logger.Warnf("failed to listen on local UDP port: %v", err)
		return
	}
	defer listener.Close()

	c.logger.Infof("UDP direct listener started successfully, listening on address: %s", listener.LocalAddr().String())

	buf := make([]byte, 16*1024)
	activeConnections := map[string]*LocalUDPConn{}
	mu := &sync.Mutex{}

	go func() {
		for {
			select {
			case <-c.ctx.Done():
				return
			default:
				n, addr, err := listener.ReadFromUDP(buf)
				if err != nil {
					continue
				}

				key := addr.String()
				mu.Lock()
				if existingConn, exists := activeConnections[key]; exists {
					select {
					case existingConn.payload <- append([]byte(nil), buf[:n]...):
					default:
					}
					mu.Unlock()
					continue
				}
				mu.Unlock()

				selected := remoteAddrs[0]
				if len(remoteAddrs) > 1 {
					srcIP, _, _ := net.SplitHostPort(addr.String())
					dstIP, _, _ := net.SplitHostPort(listener.LocalAddr().String())
					selected = utils.SelectBySrcDstHash(srcIP, dstIP, remoteAddrs)
				}

				payloadChan := make(chan []byte, 100000)
				newUDPConn := LocalUDPConn{
					timeCreated: time.Now().UnixNano(),
					payload:     payloadChan,
					remoteAddr:  selected,
					listener:    listener,
					addr:        addr,
				}

				mu.Lock()
				activeConnections[key] = &newUDPConn
				mu.Unlock()

				payloadChan <- append([]byte(nil), buf[:n]...)
				go c.handleLocalDirectConn(&newUDPConn, &activeConnections, mu)
			}
		}
	}()

	<-c.ctx.Done()
}

func (c *UdpTransport) handleLocalDirectConn(localConn *LocalUDPConn, activeConnections *map[string]*LocalUDPConn, mu *sync.Mutex) {
	remoteAddr, err := net.ResolveUDPAddr("udp", c.config.RemoteAddr)
	if err != nil {
		c.logger.Error("failed to resolve tunnel address:", err)
		c.cleanupLocalConn(localConn, activeConnections, mu)
		return
	}

	tunConn, err := net.DialUDP("udp", nil, remoteAddr)
	if err != nil {
		c.logger.Error("failed to connect to server:", err)
		c.cleanupLocalConn(localConn, activeConnections, mu)
		return
	}
	defer tunConn.Close()

	// Direct Mode Handshake: Send Token
	_, err = tunConn.Write([]byte(c.config.Token))
	if err != nil {
		c.cleanupLocalConn(localConn, activeConnections, mu)
		return
	}

	// Then send the remote address to connect to
	encodedAddr := utils.EncodeTransportStringBytes(localConn.remoteAddr, utils.SG_UDP)
	_, err = tunConn.Write(encodedAddr)
	if err != nil {
		c.cleanupLocalConn(localConn, activeConnections, mu)
		return
	}

	done := make(chan struct{})

	// Handle data from local to tunnel
	go func() {
		defer close(done)
		inactivityTimeout := 60 * time.Second
		for {
			select {
			case data, ok := <-localConn.payload:
				if !ok {
					return
				}
				packetSize := len(data)
				totalWritten := 0
				for totalWritten < packetSize {
					w, err := tunConn.Write(data[totalWritten:])
					if err != nil {
						return
					}
					totalWritten += w
				}
			case <-time.After(inactivityTimeout):
				return
			}
		}
	}()

	// Handle data from tunnel to local
	go func() {
		buf := make([]byte, 16*1024)
		for {
			tunConn.SetReadDeadline(time.Now().Add(60 * time.Second))
			n, err := tunConn.Read(buf)
			if err != nil {
				return
			}
			localConn.listener.WriteToUDP(buf[:n], localConn.addr)
		}
	}()

	<-done
	c.cleanupLocalConn(localConn, activeConnections, mu)
}

func (c *UdpTransport) cleanupLocalConn(localConn *LocalUDPConn, activeConnections *map[string]*LocalUDPConn, mu *sync.Mutex) {
	mu.Lock()
	close(localConn.payload)
	delete(*activeConnections, localConn.addr.String())
	mu.Unlock()
}
