//go:build linux
// +build linux

package transport

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"github.com/musix/backhaul/internal/config"
	"github.com/sirupsen/logrus"
	"golang.org/x/exp/rand"
)

// Global map to track TFO enabled addresses
var tfoEnabledAddresses = make(map[string]bool)
var tfoMutex sync.RWMutex

// Obfuscation configuration
type ObfuscationConfig struct {
	EnableHTTPSObfuscation     bool
	EnableWebSocketObfuscation bool
	EnableTLSFingerprinting    bool
	CustomUserAgent            string
	CustomHeaders              map[string]string
}

// Default obfuscation config
var defaultObfuscationConfig = ObfuscationConfig{
	EnableHTTPSObfuscation:     true,
	EnableWebSocketObfuscation: true,
	EnableTLSFingerprinting:    true,
	CustomUserAgent:            "",
	CustomHeaders:              make(map[string]string),
}

// Enhanced User-Agent list for better obfuscation
var enhancedUserAgents = []string{
	// Modern Chrome versions
	"Mozilla/5.0 (Windows NT10 Win64; x64) AppleWebKit/53736(KHTML, like Gecko) Chrome/120000 Safari/537.36,Mozilla/5.0 (Macintosh; Intel Mac OS X10_157) AppleWebKit/53736(KHTML, like Gecko) Chrome/120000 Safari/537.36,Mozilla/5.0(X11; Linux x864) AppleWebKit/53736(KHTML, like Gecko) Chrome/120000 Safari/537.36,Mozilla/5.0 (Windows NT10 Win64; x64) AppleWebKit/53736(KHTML, like Gecko) Chrome/119000 Safari/537.36,Mozilla/5.0 (Macintosh; Intel Mac OS X10_157) AppleWebKit/53736(KHTML, like Gecko) Chrome/119000Safari/5370.36",
	// Modern Firefox versions
	"Mozilla/5.0 (Windows NT10.0Win64 x64; rv:1210o/2010101 Firefox/1210,Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:1210o/2010101 Firefox/1210,Mozilla/5.0(X11; Linux x864; rv:1210o/2010101 Firefox/1210,Mozilla/5.0 (Windows NT10.0Win64 x64; rv:1200o/2010101Firefox/120.0",
	// Safari
	"Mozilla/5.0 (Macintosh; Intel Mac OS X10_157) AppleWebKit/6050.115(KHTML, like Gecko) Version/170.1Safari/605.1.15,Mozilla/5.0 (Macintosh; Intel Mac OS X10_157) AppleWebKit/6050.115(KHTML, like Gecko) Version/170Safari/605.10.15",
	// Edge
	"Mozilla/5.0 (Windows NT10 Win64; x64) AppleWebKit/53736(KHTML, like Gecko) Chrome/120000 Safari/537.36 Edg/120.0.00,Mozilla/5.0 (Macintosh; Intel Mac OS X10_157) AppleWebKit/53736(KHTML, like Gecko) Chrome/120000 Safari/537.36dg/120.0",
}

// Realistic paths for obfuscation
var realisticPaths = []string{"/api/v1/stream", "/cdn/assets", "/ws/chat", "/api/notifications", "/live/stream", "/api/analytics", "/cdn/static", "/api/status", "/ws/updates", "/api/metrics"}

// Simple TLS configuration for obfuscation
func getObfuscatedTLSConfig() *tls.Config {
	return &tls.Config{
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionTLS12,
		MaxVersion:         tls.VersionTLS13,
		CurvePreferences: []tls.CurveID{
			tls.X25519,
			tls.CurveP256,
		},
	}
}

// Generate realistic headers for obfuscation
func generateObfuscatedHeaders(token string, randomUserID int32, customHeaders map[string]string) http.Header {
	headers := http.Header{}

	// Basic headers
	headers.Add("Authorization", fmt.Sprintf("Bearer %v", token))
	headers.Add("X-User-Id", fmt.Sprintf("%d", randomUserID))

	// Random User-Agent
	randomUserAgent := enhancedUserAgents[rand.Intn(len(enhancedUserAgents))]
	headers.Add("User-Agent", randomUserAgent)

	// Realistic headers that appear in normal HTTPS traffic
	headers.Add("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
	headers.Add("Accept-Language", "en-US,en;q=0.5")
	headers.Add("Accept-Encoding", "gzip, deflate, br")
	headers.Add("DNT", "1")
	// Note: Connection header is set by WebSocket library automatically
	headers.Add("Upgrade-Insecure-Requests", "1")
	headers.Add("Sec-Fetch-Dest", "document")
	headers.Add("Sec-Fetch-Mode", "navigate")
	headers.Add("Sec-Fetch-Site", "none")
	headers.Add("Cache-Control", "max-age=0")

	// Add custom headers if provided
	for key, value := range customHeaders {
		headers.Add(key, value)
	}

	return headers
}

// Generate realistic WebSocket path
func generateObfuscatedPath(basePath string, randomUserID int32) string {
	if basePath == "/channel" {
		// Use a realistic path for control channel
		realisticPath := realisticPaths[rand.Intn(len(realisticPaths))]
		return fmt.Sprintf("%s/%d", realisticPath, randomUserID)
	}
	return fmt.Sprintf("%s/%d", basePath, randomUserID)
}

func ResolveRemoteAddr(remoteAddr string) (int, string, error) {
	// Split the address into host and port
	parts := strings.Split(remoteAddr, ":")
	var port int
	var err error

	// Handle cases where only the port is sent or host:port format
	if len(parts) < 2 {
		port, err = strconv.Atoi(parts[0])
		if err != nil {
			return 0, "", fmt.Errorf("invalid port format: %v", err)
		}
		// Default to localhost if only the port is provided
		return port, fmt.Sprintf("127.0.0.1:%d", port), nil
	}

	// If both host and port are provided
	port, err = strconv.Atoi(parts[1])
	if err != nil {
		return 0, "", fmt.Errorf("invalid port format: %v", err)
	}

	// Return the full resolved address
	return port, remoteAddr, nil
}

func TcpDialer(ctx context.Context, address string, timeout time.Duration, keepAlive time.Duration, nodelay bool, retry int, SO_RCVBUF int, SO_SNDBUF int, logger *logrus.Logger) (*net.TCPConn, error) {
	var tcpConn *net.TCPConn
	var err error

	retries := retry           // Number of retries
	backoff := 1 * time.Second // Initial backoff duration

	for i := 0; i < retries; i++ {
		// Attempt to establish a TCP connection
		tcpConn, err = attemptTcpDialer(ctx, address, timeout, keepAlive, nodelay, SO_RCVBUF, SO_SNDBUF, logger)
		if err == nil {
			// Connection successful
			return tcpConn, nil
		}

		// If this is the last retry, return the error
		if i == retries-1 {
			break
		}

		// Log retry attempt and wait before retrying
		time.Sleep(backoff)
		backoff *= 2 // Exponential backoff (double the wait time after each failure)
	}

	return nil, err
}

func attemptTcpDialer(ctx context.Context, address string, timeout time.Duration, keepAlive time.Duration, nodelay bool, SO_RCVBUF int, SO_SNDBUF int, logger *logrus.Logger) (*net.TCPConn, error) {
	//Resolve the address to a TCP address
	tcpAddr, err := net.ResolveTCPAddr("tcp", address)
	if err != nil {
		return nil, fmt.Errorf("DNS resolution: %v", err)
	}

	// Options
	dialer := &net.Dialer{
		Timeout:   timeout,   // Set the connection timeout
		KeepAlive: keepAlive, // Set the keep-alive duration
	}

	// Add socket options for Linux
	if runtime.GOOS == "linux" {
		dialer.Control = func(network, address string, s syscall.RawConn) error {
			var controlErr error
			err := s.Control(func(fd uintptr) {
				sockFd := int(fd)

				// Set SO_RCVBUF
				if SO_RCVBUF > 0 {
					if err := syscall.SetsockoptInt(sockFd, syscall.SOL_SOCKET, syscall.SO_RCVBUF, SO_RCVBUF); err != nil {
						controlErr = fmt.Errorf("failed to set SO_RCVBUF: %v", err)
						return
					}
				}

				// Set SO_SNDBUF
				if SO_SNDBUF > 0 {
					if err := syscall.SetsockoptInt(sockFd, syscall.SOL_SOCKET, syscall.SO_SNDBUF, SO_SNDBUF); err != nil {
						controlErr = fmt.Errorf("failed to set SO_SNDBUF: %v", err)
						return
					}
				}

				// TCP Fast Open
				if err := syscall.SetsockoptInt(sockFd, syscall.IPPROTO_TCP, 23 /* TCP_FASTOPEN */, 1); err != nil {
					controlErr = fmt.Errorf("failed to set TCP_FASTOPEN: %v", err)
					return
				}
				// Log TFO enablement only once per address
				tfoMutex.Lock()
				if !tfoEnabledAddresses[address] {
					logger.Infof("TCP Fast Open enabled for %s", address)
					tfoEnabledAddresses[address] = true
				}
				tfoMutex.Unlock()
			})
			if err != nil {
				return err
			}
			return controlErr
		}
	}

	// Dial the TCP connection with a timeout
	conn, err := dialer.DialContext(ctx, "tcp", tcpAddr.String())
	if err != nil {
		return nil, err
	}

	// Type assert the net.Conn to *net.TCPConn
	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		conn.Close()
		return nil, fmt.Errorf("failed to convert net.Conn to *net.TCPConn")
	}

	if !nodelay {
		err = tcpConn.SetNoDelay(false)
		if err != nil {
			tcpConn.Close()
			return nil, fmt.Errorf("failed to set TCP_NODELAY")
		}
	}

	return tcpConn, nil
}

func WebSocketDialer(ctx context.Context, addr string, edgeIP string, path string, timeout time.Duration, keepalive time.Duration, nodelay bool, token string, mode config.TransportType, retry int, SO_RCVBUF int, SO_SNDBUF int, logger *logrus.Logger) (*websocket.Conn, error) {

	var tunnelWSConn *websocket.Conn
	var err error

	retries := retry           // Number of retries
	backoff := 1 * time.Second // Initial backoff duration

	for i := 0; i < retries; i++ {
		// Attempt to dial the WebSocket
		tunnelWSConn, err = attemptDialWebSocket(ctx, addr, edgeIP, path, timeout, keepalive, nodelay, token, mode, SO_RCVBUF, SO_SNDBUF, logger)
		if err == nil {
			// If successful, return the connection
			return tunnelWSConn, nil
		}

		// If this is the last retry, return the error
		if i == retries-1 {
			break
		}

		// Log the retry attempt and wait before retrying
		time.Sleep(backoff)
		backoff *= 2 // Exponential backoff (double the wait time after each failure)
	}

	return nil, err
}

func attemptDialWebSocket(ctx context.Context, addr string, edgeIP string, path string, timeout time.Duration, keepalive time.Duration, nodelay bool, token string, mode config.TransportType, SO_RCVBUF int, SO_SNDBUF int, logger *logrus.Logger) (*websocket.Conn, error) {
	// Generate a random X-user-id
	rand.Seed(uint64(time.Now().UnixNano()))
	randomUserID := rand.Int31() // Generate a random int32 number

	// Generate obfuscated headers
	headers := generateObfuscatedHeaders(token, randomUserID, defaultObfuscationConfig.CustomHeaders)

	var wsURL string
	dialer := websocket.Dialer{}

	// Handle edgeIP assignment
	if edgeIP != "" {
		_, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, fmt.Errorf("invalid address format, failed to parse: %w", err)
		}

		edgeIP = fmt.Sprintf("%s:%s", edgeIP, port)
	} else {
		edgeIP = addr
	}

	// Generate obfuscated path
	obfuscatedPath := generateObfuscatedPath(path, randomUserID)

	switch mode {
	case config.WS, config.WSMUX:
		wsURL = fmt.Sprintf("ws://%s%s", addr, obfuscatedPath)
		dialer = websocket.Dialer{
			EnableCompression: true,
			HandshakeTimeout:  45 * time.Second, // default handshake timeout
			NetDial: func(_, addr string) (net.Conn, error) {
				conn, err := TcpDialer(ctx, edgeIP, timeout, keepalive, nodelay, 1, SO_RCVBUF, SO_SNDBUF, logger) // Pass nil for logger as it's not used in this context
				if err != nil {
					return nil, err
				}
				return conn, nil
			},
		}
	case config.WSS, config.WSSMUX:
		wsURL = fmt.Sprintf("wss://%s%s", addr, obfuscatedPath)
		// Use obfuscated TLS configuration
		tlsConfig := getObfuscatedTLSConfig()
		dialer = websocket.Dialer{
			EnableCompression: true,
			TLSClientConfig:   tlsConfig,
			HandshakeTimeout:  45 * time.Second, // default handshake timeout
			NetDial: func(_, addr string) (net.Conn, error) {
				conn, err := TcpDialer(ctx, edgeIP, timeout, keepalive, nodelay, 1, SO_RCVBUF, SO_SNDBUF, logger)
				if err != nil {
					return nil, err
				}
				return conn, nil
			},
		}
	default:
		return nil, fmt.Errorf("unsupported transport mode: %v", mode)
	}

	// Dial to the WebSocket server
	tunnelWSConn, _, err := dialer.Dial(wsURL, headers)
	if err != nil {
		return nil, err
	}
	return tunnelWSConn, nil
}
