# Client Whitelist Configuration

## Overview

The client whitelist feature allows you to restrict tunnel connections to only specific clients based on their IP addresses or IP ranges (CIDR).

## Configuration

Add the `allowed_clients` field to the server configuration:

```toml
[server]
bind_addr = "0.0.0.0:8080"
transport = "tcp"
token = "your_secure_token"

# Whitelist of allowed clients
# If empty or not specified, all clients are allowed
allowed_clients = [
    "192.168.1.100",           # Specific IPv4 address
    "192.168.1.0/24",          # CIDR range (IPv4)
    "10.0.0.1",                # Another specific IPv4
    "::1",                     # IPv6 address (localhost)
    "2001:db8::1",             # Another IPv6 address
    "2001:db8::/32",           # IPv6 CIDR range
]
```

## Supported Formats

### 1. IPv4 Addresses
```toml
allowed_clients = [
    "192.168.1.100",
    "10.0.0.1",
    "172.16.0.50"
]
```

### 2. IPv4 CIDR Ranges
```toml
allowed_clients = [
    "192.168.0.0/16",          # All addresses in 192.168.x.x
    "10.0.0.0/8",              # All addresses in 10.x.x.x
    "172.16.0.0/12"            # All addresses in 172.16.x.x - 172.31.x.x
]
```

### 3. IPv6 Addresses
IPv6 addresses can be specified with or without brackets:
```toml
allowed_clients = [
    "::1",                     # IPv6 localhost
    "2001:db8::1",             # Global IPv6
    "fe80::1"                  # Link-local IPv6
]
```

### 4. IPv6 CIDR Ranges
```toml
allowed_clients = [
    "2001:db8::/32",           # IPv6 range
    "fe80::/10"                # Link-local range
]
```

### 5. Mixed Configuration
```toml
allowed_clients = [
    "192.168.1.100",           # Single IPv4
    "10.0.0.0/24",             # IPv4 range
    "::1",                     # IPv6
    "2001:db8::/32"            # IPv6 range
]
```

## Behavior

### When Whitelist is Empty or Not Configured
- All clients are allowed to connect
- No filtering is applied

### When Whitelist is Configured with Entries
- Only clients with IPs matching the whitelist are allowed
- Connections from non-whitelisted clients are rejected with a warning log
- Rejected connections are closed immediately

### Connection Rejection Behavior

Different transports handle rejection differently:

**TCP/TCP-MUX**: Connection is closed immediately with warning log
```
WARN client <ip:port> is not in whitelist, rejecting connection
```

**UDP**: Packet is dropped with warning log
```
WARN client <ip:port> is not in whitelist, rejecting UDP connection
```

**WebSocket/WebSocket-MUX**: HTTP 403 Forbidden response
```
WARN client <ip:port> is not in whitelist, rejecting connection
```

**QUIC**: Connection is closed with error code 0x0403
```
WARN client <ip:port> is not in whitelist, rejecting connection
```

## Examples

### Example 1: Office Network Only
```toml
[server]
allowed_clients = [
    "203.0.113.0/24",          # Office subnet
]
```

### Example 2: Multiple Offices
```toml
[server]
allowed_clients = [
    "203.0.113.0/24",          # Main office
    "198.51.100.0/24",         # Branch office
    "192.168.1.50"             # Specific client
]
```

### Example 3: Mixed IPv4 and IPv6
```toml
[server]
allowed_clients = [
    "192.168.1.0/24",          # IPv4 LAN
    "2001:db8:1::/48",         # IPv6 LAN
    "10.0.0.1"                 # Specific IPv4
]
```

### Example 4: Single Client
```toml
[server]
allowed_clients = [
    "192.168.1.50"             # One specific client
]
```

## Performance Considerations

- **IP Address Matching**: O(n) where n is number of entries
- **CIDR Matching**: O(n) with subnet calculations
- **Recommendation**: Order frequently used entries first for optimal performance

## Logging

When a client is rejected, the server logs:
```
WARN client 192.168.1.200:54321 is not in whitelist, rejecting connection
```

To view all whitelist rejections, enable debug logging:
```toml
[server]
log_level = "debug"
```

## Security Notes

1. **Exact IP Matching**: Single IP entries match exactly and are case-sensitive.

2. **CIDR Ranges**: Use appropriate subnet masks to avoid unintended broad access:
   - `/24` = 256 addresses (typical LAN)
   - `/16` = 65,536 addresses (large network)
   - `/8` = 16+ million addresses (entire class A)

3. **IPv6 Format**: IPv6 addresses can be specified with or without brackets:
   ```toml
   allowed_clients = [
       "::1",              # Without brackets (recommended for config)
       "2001:db8::1"       # Without brackets
   ]
   ```
   Internally, the system handles both `::1` and `[::1]` formats correctly.

## Troubleshooting

### All Clients Are Rejected
- Check TOML syntax: ensure `allowed_clients` is a proper array
- Verify IP addresses match client's actual IP (check server logs for client IP)
- Test with direct IP address
- Validate CIDR syntax: must be `/` followed by prefix length (0-32 for IPv4, 0-128 for IPv6)

### Legitimate Client Blocked
1. Check server logs for the client's actual IP address and port
2. Verify the IP is in the whitelist (or matches a CIDR range)
3. Test with a simple single IP entry first
4. Check for typos in IP addresses

### Empty Whitelist Not Working
If `allowed_clients = []` doesn't allow all clients:
- Remove the `allowed_clients` line entirely
- Don't set it to an empty array; omitting it is better

## Combining with Token Authentication

Client whitelist works alongside token authentication:
1. **Whitelist Check**: IP is checked first (faster)
2. **Token Check**: Authentication token is verified after whitelist passes

```toml
[server]
token = "secure_token_here"
allowed_clients = [
    "192.168.1.0/24"
]
```

Both checks must pass for connection to succeed:
- ✅ Correct IP + Correct Token = Connected
- ❌ Correct IP + Wrong Token = Rejected
- ❌ Wrong IP + Correct Token = Rejected
- ❌ Wrong IP + Wrong Token = Rejected

## TOML Validation

Ensure your configuration is valid TOML:

```toml
# Valid - single IP
allowed_clients = ["192.168.1.100"]

# Valid - multiple IPs
allowed_clients = [
    "192.168.1.100",
    "10.0.0.1",
    "192.168.1.0/24"
]

# Invalid - strings not quoted
allowed_clients = [192.168.1.100]

# Invalid - missing array brackets
allowed_clients = "192.168.1.100"

# Valid - proper array format with CIDR
allowed_clients = [
    "192.168.1.100",
    "10.0.0.0/8"
]
```

## IP Address Extraction

The system extracts IP addresses from client connections in the following formats:
- **IPv4 with port**: `192.168.1.1:8080` → extracts `192.168.1.1`
- **IPv6 with port**: `[2001:db8::1]:8080` → extracts `2001:db8::1`
- **IPv6 without port**: `2001:db8::1` → uses as-is
- **IPv4 without port**: `192.168.1.1` → uses as-is

## Validation at Startup

When the server starts, CIDR ranges are validated:
- Invalid formats will cause an error and server will not start
- Example error: `invalid CIDR range '192.168.1.0/33': invalid CIDR address`

Valid CIDR ranges:
- **IPv4**: Prefix length 0-32 (example: `192.168.1.0/24`)
- **IPv6**: Prefix length 0-128 (example: `2001:db8::/32`)

## FAQ

**Q: Can I use domain names?**  
A: No, the whitelist only supports IP addresses and CIDR ranges. Use IP addresses of your clients.

**Q: What if a client has a dynamic IP?**  
A: Use a CIDR range that covers the client's IP range, or use a firewall rule outside of Backhaul.

**Q: Can I use wildcards like `192.168.1.*`?**  
A: No, use CIDR notation instead: `192.168.1.0/24`

**Q: How does IPv6 matching work?**  
A: Exactly like IPv4. You can use full addresses (`2001:db8::1`) or ranges (`2001:db8::/32`).

