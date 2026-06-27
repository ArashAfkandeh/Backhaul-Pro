package transport

import (
	"context"
	"time"

	"github.com/musix/backhaul/internal/utils"
	"github.com/musix/backhaul/internal/client/transport"
	"github.com/quic-go/quic-go"
)

func (s *QuicTransport) handleSessionDirect(session quic.Connection) {
	for {
		select {
		case <-s.ctx.Done():
			return
		default:
			stream, err := session.AcceptStream(context.Background())
			if err != nil {
				s.logger.Trace("session is closed: ", err)
				return
			}

			go func(stm quic.Stream) {
				remoteAddr, tunTransport, err := utils.ReceiveBinaryTransportString(stm)
				if err != nil {
					s.logger.Debugf("failed to receive remote addr from stream: %v", err)
					stm.Close()
					return
				}
				port, resolvedAddr, err := transport.ResolveRemoteAddr(remoteAddr)
				if err != nil {
					stm.Close()
					return
				}

				switch tunTransport {
				case utils.SG_TCP:
					localConnection, err := transport.TcpDialer(s.ctx, resolvedAddr, 10*time.Second, s.config.KeepAlive, true, 1, 32*1024, 32*1024, s.logger)
					if err != nil {
						stm.Close()
						return
					}
					utils.QConnectionHandler(localConnection, stm, s.logger, s.usageMonitor, port, s.config.Sniffer)
				case utils.SG_UDP:
					// Not implemented for QConnectionHandler UDP
					stm.Close()
				default:
					stm.Close()
				}
			}(stream)
		}
	}
}
