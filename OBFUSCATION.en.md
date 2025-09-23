# Backhaul Obfuscation Features

## Overview

Backhaul now includes advanced obfuscation features to bypass DPI (Deep Packet Inspection) and censorship systems. These features make the traffic appear as legitimate HTTPS/WebSocket traffic from real browsers.

## Obfuscation Layers

### 1 HTTPS Traffic Obfuscation

The client now generates realistic HTTP headers that mimic real browser traffic:

- **User-Agent Rotation**: Randomly selects from a pool of real browser User-Agent strings
- **Realistic Headers**: Includes standard headers like `Accept`, `Accept-Language`, `Accept-Encoding`, `DNT`, `Connection`, etc.
- **Security Headers**: Adds modern security headers like `Sec-Fetch-*` headers
- **Custom Headers**: Supports custom headers for additional obfuscation

###2. WebSocket Path Obfuscation

Instead of using predictable paths like `/channel`, the client now uses realistic paths:

- **Realistic Paths**: Uses paths like `/api/v1/stream`, `/cdn/assets`, `/ws/chat`, etc.
- **Dynamic Paths**: Each connection gets a unique path with a random user ID
- **Server Compatibility**: Server automatically detects and handles obfuscated paths

### 3. TLS Fingerprinting Obfuscation

Enhanced TLS configuration to avoid fingerprinting:

- **Modern TLS Versions**: Uses TLS 1.2 and 1.3 **Standard Curves**: Uses X25519 and P-256 curves
- **Browser-like Configuration**: Mimics real browser TLS settings

## Configuration

### Default Settings

Obfuscation is enabled by default with the following configuration:

```go
var defaultObfuscationConfig = ObfuscationConfig[object Object]    EnableHTTPSObfuscation: true,
    EnableWebSocketObfuscation: true,
    EnableTLSFingerprinting: true,
    CustomUserAgent: "",
    CustomHeaders: make(map[string]string),
}
```

### User-Agent Pool

The system includes a diverse set of real User-Agent strings from:

- **Chrome**: Multiple versions across Windows, macOS, Linux, and Android
- **Firefox**: Various versions and platforms
- **Safari**: macOS and iOS versions
- **Edge**: Windows and macOS versions

### Realistic Paths

Available obfuscated paths include:

- `/api/v1/stream`
- `/cdn/assets`
- `/ws/chat`
- `/api/notifications`
- `/live/stream`
- `/api/analytics`
- `/cdn/static`
- `/api/status`
- `/ws/updates`
- `/api/metrics`

## How It Works

### Client Side

1. **Connection Initiation**: When establishing a WebSocket connection, the client:
   - Generates a random user ID
   - Selects a random User-Agent from the pool
   - Creates realistic HTTP headers
   - Uses an obfuscated path instead of `/channel`

2**TLS Handshake**: For WSS connections:
   - Uses modern TLS configuration
   - Implements browser-like curve preferences
   - Avoids fingerprinting detection

### Server Side

1. **Path Detection**: The server automatically detects obfuscated paths
2. **Header Processing**: Processes all realistic headers while maintaining security
3**Compatibility**: Works with both obfuscated and non-obfuscated clients

## Benefits

### Anti-Detection

- **DPI Evasion**: Traffic appears as legitimate web traffic
- **Fingerprint Avoidance**: Avoids TLS and HTTP fingerprinting
- **Behavioral Obfuscation**: Mimics real browser behavior patterns

### Flexibility

- **Backward Compatibility**: Works with existing non-obfuscated clients
- **Configurable**: Can be customized for specific environments
- **Extensible**: Easy to add new obfuscation techniques

## Security Considerations

### Advantages

- **Censorship Resistance**: Better chance of bypassing content filters
- **Privacy Enhancement**: Reduces traffic pattern analysis
- **Stealth Operation**: Makes detection more difficult

### Limitations

- **Not Perfect**: Advanced DPI systems may still detect the traffic
- **Performance**: Minimal overhead from obfuscation
- **Maintenance**: User-Agent strings need periodic updates

## Usage Examples

### Basic Usage

The obfuscation features work automatically when using WebSocket transports:

```toml
[client]
transport =wss"
remote_addr = example.com:443oken = your_token"
```

### Custom Headers

You can add custom headers for additional obfuscation:

```go
customHeaders := map[string]string[object Object]
    X-Forwarded-For:2030131,
    X-Real-IP:23.0.113.1   CF-Connecting-IP:20300.113```

## Monitoring and Logging

### Client Logs

The client logs obfuscation activities at trace level:

```
TRACE detected obfuscated path: /api/v1/stream/12345TRACE using User-Agent: Mozilla/5.0 (Windows NT10; Win64 x64) ...
```

### Server Logs

The server logs detected obfuscated paths:

```
TRACE received request for path: /api/v1/stream/12345
TRACE detected obfuscated path: /api/v1/stream/12345
## Future Enhancements

### Planned Features
1 **Domain Fronting**: Support for CDN-based obfuscation2Protocol Mimicking**: Emulate other protocols (HTTP/2, gRPC)
3. **Traffic Shaping**: Add realistic traffic patterns
4ertificate Pinning**: Support for custom certificates
5 **Multi-layer Obfuscation**: Combine multiple techniques

### Contributing

To add new obfuscation techniques:

1Update the `ObfuscationConfig` struct
2. Implement the obfuscation logic
3 appropriate tests
4. Update this documentation

## Conclusion

The obfuscation features significantly improve Backhaul's ability to operate in restrictive network environments while maintaining performance and compatibility. The multi-layer approach provides robust protection against various detection methods. 