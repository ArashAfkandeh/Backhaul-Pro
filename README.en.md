## Backhaul

A high-performance reverse tunneling system to traverse NAT and firewalls, supporting TCP/WS/WSS and multiplexed modes, with a built-in monitoring web panel, optional sniffer, and automatic performance tuning.

- Version: v0.6.6
- Language: Go 1.23+
- GitHub repository: [Backhaul-Pro](https://github.com/ArashAfkandeh/Backhaul-Pro.git)
- License: AGPL-3.0

### Table of Contents
- Introduction & Architecture
- Features & Advantages
- Transports & Use Cases
- Security & Authentication
- Web Panel & Monitoring APIs
- Automatic Tuning (Auto-Tune)
- Hot Reload of configuration
- Install & Upgrade (using installer.sh)
- Manual Build from Source
- Service (systemd) & Service Management
- Example Server/Client Configs with Auto-Tune enabled
- Operational Notes & Troubleshooting
- Code Structure & Important Files
- License
- Obfuscation (Traffic Camouflage)
- Self-Signed Certificate Generation
- PPROF
- Detailed Auto-Tune
- Hot Reload (safe)
- Installer Script – Full
- Minor/Hidden Capabilities

---

### Introduction & Architecture
Backhaul is designed for secure and scalable traffic tunneling from behind NAT/firewalls. A single binary runs in server or client mode based on the content of `config.toml`.

- Main process (`main.go`):
  - Parses flags (`-c`, `-v`, `--no-auto-tune`, `--tune-interval`)
  - Applies temporary TCP tuning on OS startup
  - Starts main logic via `cmd.Run`
  - Launches dynamic Tuner (if enabled)
  - Hot Reload on configuration file changes
  - Graceful shutdown and force-shutdown handling

- Command module (`cmd/`):
  - Loads TOML, applies defaults (`cmd/defaults.go`)
  - Chooses role: server (`internal/server`) or client (`internal/client`)

- Transport stack:
  - TCP, TCPMUX, WS/WSS, WSMUX/WSSMUX, UDP
  - Multiplexing via SMUX (configure `mux_*` parameters)

- Web panel & Sniffer (`internal/web/`):
  - Built-in HTML dashboard (Tailwind)
  - Endpoints: `/`, `/stats`, `/data`, `/config`
  - Stores and serves per-port usage reports as JSON

---

### Features & Advantages
- High performance for massive concurrency
- Multiple transports: `tcp`, `tcpmux`, `ws`, `wss`, `wsmux`, `wssmux`, `udp`
- Multiplexing (SMUX) for multiple logical streams over one connection
- Monitoring web panel with live system/tunnel stats and current config
- Optional sniffer: per-port usage logs as readable JSON
- Auto-Tune: dynamic adjustment of `keepalive`, `mux_*`, `channel_size`, `connection_pool`, `heartbeat`, `mux_con`
- Hot Reload: safe application of config changes
- PPROF: optional profiling on ports 6060/6061 (Server/Client)

Note: QUIC support is removed in this project. Focus is on TCP/WS/WSS (+MUX) and UDP.

---

### Transports & Use Cases
- TCP: simple and fast; best for stable networks
- TCPMUX: fewer physical connections via multiplexed streams
- WS/WSS: traverses HTTP-layer proxies/firewalls; WSS is TLS-secured
- WSMUX/WSSMUX: WS/WSS with multiplexing
- UDP: tunnel UDP (and/or accept UDP over TCP on server with `accept_udp`)

Use cases:
- Bypass restrictions using `wss` on public ports 443/8443
- High-connection density with `tcpmux` or `wsmux/wssmux` and tuned `mux_*`
- HTTP-restricted environments with `ws/wss` and `edge_ip` (CDN-friendly)

---

### Security & Authentication
- Token: all tunnel requests are authenticated with `token`. Use a strong random value.
- TLS (WSS/WSSMUX only): use a valid certificate in production. Self-signed generation samples are provided below.
- Web panel: restrict access (IP whitelist, firewall, reverse proxy) or bind to a local interface.

---

### Web Panel & Monitoring APIs
Enabled when `web_port > 0`.
- `/` HTML dashboard with current config, tunnel status, and system stats
- `/stats` JSON: CPU/RAM/Disk/Swap/Traffic/BackhaulTraffic/Connections/Status
- `/data` JSON of per-port usage (only if `sniffer=true`)
- `/config` current config without sensitive fields; `?type=client` returns client config

Client-side dynamic sync:
- Client periodically syncs some parameters (e.g., `keepalive_period`, `mux_*`) from server `/config`.

---

### Automatic Tuning (Auto-Tune)
Enabled by default. Disable with `--no-auto-tune`. Tuning interval is configurable via `--tune-interval` (default 10m).
- Inputs: CPU, RAM, network latency (TCP dial), packet loss, throughput
- Adjustments:
  - Client: `connection_pool`
  - Server: `channel_size`, `mux_*` (framesize/receivebuffer/streambuffer), `heartbeat`, `mux_con`
  - Both: `keepalive_period` (kept in sync)
- Interval adapts to network stability (variance/average)

---

### Hot Reload of configuration
The `-c` config file is watched; when its mtime changes:
- Gracefully stop previous instance (cancel context) and start a new one
- Stop/restart Tuner if enabled

---

### Install & Upgrade (using installer.sh)
The interactive installer automates online/offline setup, systemd service creation, config creation/editing, and centralized management.

- Download/run (Debian/Ubuntu with sudo):
```bash
curl -LO https://raw.githubusercontent.com/ArashAfkandeh/Backhaul-Pro/main/installer.sh
sudo bash installer.sh
```

- Run directly without saving:
```bash
bash <(curl -fsSL https://raw.githubusercontent.com/ArashAfkandeh/Backhaul-Pro/main/installer.sh)
```

- Run directly with arguments (no file saved):
```bash
# Uninstall all or selected services via interactive menu
bash <(curl -fsSL https://raw.githubusercontent.com/ArashAfkandeh/Backhaul-Pro/main/installer.sh) uninstall

# Open central management menu (status/logs/restart/edit/uninstall)
bash <(curl -fsSL https://raw.githubusercontent.com/ArashAfkandeh/Backhaul-Pro/main/installer.sh) manage
```

- Modes:
  - Online: installs dependencies via apt, downloads binary package, extracts to `/root/backhaul_pro`
  - Offline: installs from local archive `/root/backhaul_pro.tar.gz` and local apt repo

- Key steps:
  1) System prep & dependencies (wget, curl, openssl, tar, net-tools)
  2) Create `/root/backhaul_pro`
  3) Extract package and place `backhaul_pro` binary
  4) Choose role (Server/Client), tunnel port, protocol (udp/tcp/tcpmux/ws/wss/wsmux/wssmux), token (random/custom), `web_port`
  5) (Server) define `ports` with validation and local port occupancy checks
  6) Create `config.toml` for selected role
  7) Create systemd service like `backhaul_pro.service` (numbered if multiple configs)
  8) Enable/start service and show status
  9) Install central management tool `bh-p` to `/usr/local/bin`

- Manage services with `bh-p`:
  - List services, status/live logs, restart
  - Interactive config editing with auto-apply
  - Server connection info viewer
  - Uninstall selected service with safe cleanup

- Uninstall services:
```bash
sudo bash installer.sh uninstall
# or central manager
sudo bh-p
```
Note: If `config.toml` exists, the installer can replace or create a numbered config and handle the related service.

---

### Manual Build from Source
```bash
git clone https://github.com/ArashAfkandeh/Backhaul-Pro.git
cd Backhaul-Pro
go build -o backhaul_pro
./backhaul_pro -c /path/to/config.toml
```
- Version: `./backhaul_pro -v`
- Disable Auto-Tune: `--no-auto-tune`
- Change tuning interval: `--tune-interval 15m`

---

### Service (systemd) & Management
The installer creates a service file. If you need a manual example:
```ini
[Unit]
Description=Backhaul Pro Reverse Tunnel Service
After=network.target

[Service]
Type=simple
ExecStart=/root/backhaul_pro/backhaul_pro -c /root/backhaul_pro/config.toml
Restart=always
RestartSec=3
LimitNOFILE=1048576

[Install]
WantedBy=multi-user.target
```
Common commands:
```bash
sudo systemctl daemon-reload
sudo systemctl enable backhaul_pro.service
sudo systemctl start backhaul_pro.service
sudo systemctl status backhaul_pro.service
sudo journalctl -u backhaul_pro.service -f
```

---

### Example Server/Client Configs with Auto-Tune enabled
Auto-Tune is enabled by default (10 minutes). Just run without `--no-auto-tune`. Web panel is enabled in examples.

- Server (WSSMUX on 443 with multiplexing):
```toml
[server]
bind_addr = "0.0.0.0:443"
transport = "wssmux"
token = "YOUR_TOKEN"
web_port = 2060

# Port mappings
ports = [
  "443-600",
  "443-600:5201",
  "443-600=1.1.1.1:5201",
]
```
Run server (10m interval):
```bash
./backhaul_pro -c /root/backhaul_pro/config.toml --tune-interval 10m
```

- Client (WSSMUX with server-side sync):
```toml
[client]
remote_addr = "SERVER_IP:443"
transport = "wssmux"
token = "YOUR_TOKEN"
web_port = 2061

# Optional for CDN scenarios in WS/WSS
edge_ip = ""
```
Run client (10m interval):
```bash
./backhaul_pro -c /root/backhaul_pro/config.toml --tune-interval 10m
```
Note: Auto-Tune runs on both ends; keepalive is synchronized. Disable with `--no-auto-tune`.

---

### Operational Notes & Troubleshooting
- Use a strong token and distribute it securely
- Restrict web panel exposure or bind it to a local interface
- On public ports (443) prefer WSS/WSSMUX with a valid certificate
- If nothing runs, verify `-c` path; check logs
- For high latency/variance, enable MUX and allow Auto-Tune to adapt `mux_*`
- If a port is busy, the installer will report it; choose a different port

---

### Self-Signed Certificate (for testing WSS/WSSMUX only)
```bash
openssl genpkey -algorithm RSA -out server.key -pkeyopt rsa_keygen_bits:2048
openssl req -new -key server.key -out server.csr
openssl x509 -req -in server.csr -signkey server.key -out server.crt -days 365
```

---

### Code Structure & Important Files
- `main.go`: program entry, signals, Hot Reload, Tuner
- `cmd/`: config load/validation, defaults, role selection
- `internal/config/`: config types and transport enums
- `internal/server`, `internal/client`: start transports by selected type
- `internal/*/transport`: implementations for `tcp`, `tcpmux`, `ws/wss`, `wsmux/wssmux`, `udp`
- `internal/web/`: dashboard and APIs (`/`, `/stats`, `/data`, `/config`)
- `internal/tuning/`: dynamic tuning logic and parameter synchronization
- `internal/utils/logger.go`: colored logger

---

### License
This project is licensed under AGPL-3.0. See `LICENSE` for details.

---

### Obfuscation (Traffic Camouflage)
For DPI/censorship evasion in WS/WSS/W(S)MUX modes, multiple layers of HTTP/TLS/path obfuscation are applied to resemble real browser traffic:
- Realistic HTTP headers (Accept, Accept-Language, Accept-Encoding, DNT, Connection, ...)
- User-Agent rotation across common browsers/platforms
- Realistic and dynamic WebSocket paths (e.g. `/api/v1/stream`, `/cdn/assets`, `/ws/chat`, ...)
- TLS settings similar to modern browsers (TLS 1.2/1.3, curves X25519 and P-256)
- Optional custom headers in WS client implementation

Pros:
- Looks like ordinary HTTPS/WS; lowered DPI detection risk
- Flexible and extensible patterns
- Backward compatible with non-obfuscated clients

Cons:
- No obfuscation is perfect; advanced DPI may still detect
- Small overhead; UA pool requires periodic updates

See `OBFUSCATION.en.md` for more details.

---

### Automatic Self-Signed TLS Defaults
For testing or when a valid cert isn’t ready, generate a self-signed pair (commands above). If TLS paths are left empty on server, defaults are auto-set to:
- working-dir/ssl/server.crt
- working-dir/ssl/server.key

Use valid certificates in production and restrict key permissions.

---

### PPROF
Enable profiling via `PPROF = true`:
- Server: `0.0.0.0:6060`
- Client: `0.0.0.0:6061`
Use standard Go/pprof tools or a browser. Only enable in secure environments.

---

### Auto-Tune Details
Runs in `internal/tuning/tuner.go` at a periodic interval (configurable via `--tune-interval`). It measures:
- CPU load, RAM usage
- Network latency (TCP dial), with history (avg/variance)
- Packet loss and approximate throughput

Adjusts:
- Client: `connection_pool`
- Server: `channel_size`, `mux_framesize`, `mux_recievebuffer`, `mux_streambuffer`, `heartbeat`, `mux_con`
- Both: `keepalive_period` (client-side is also kept in sync through web panel)

Adaptive interval:
- High latency variance → shorter interval (faster tuning)
- Stable network → longer interval (lower overhead)

---

### Hot Reload (safe)
Watches the `-c` file. On modification:
1) Stops current Tuner if enabled
2) Cancels previous context and starts a fresh instance
3) Restarts Tuner (if enabled) with updated config

If graceful shutdown exceeds 5 seconds, a force shutdown is applied.

---

### Installer Script (`installer.sh`) – Full
Capabilities:
- Online/Offline dependency installation (apt) and offline repo management
- Download/extract binary and create `/root/backhaul_pro` layout
- Create/edit `config.toml` (Server/Client) with IP/Port validation and local port occupancy checks
- Protocol selection: `udp`, `tcp`, `tcpmux`, `ws`, `wss`, `wsmux`, `wssmux`
- Random/custom token generation
- Set `web_port` and define `ports` for server (patterns like `443-600`, `443-600:5201`, `127.0.0.2:443=1.1.1.1:5201`, ...)
- Create systemd service (numbered for multiple configs)
- Start/enable service and show status
- Install central management tool `bh-p`

Central management:
```bash
sudo bh-p
```
Menu actions:
- Show status, live logs, restart
- Interactive config edit with auto-apply
- Show server connection info
- Uninstall selected service with safe cleanup of files and service

Batch uninstall:
```bash
sudo bash installer.sh uninstall
```
The script detects `backhaul_pro*.service` and `utunnel*.service`. If no configs/binaries remain, it can remove the working directory. If no related services remain, it also removes `bh-p`.

---

### Minor/Hidden Capabilities
- `accept_udp` (TCP server): forward UDP over TCP tunnel
- `channel_size` (server): queue capacity; drops/controls overload
- `connection_pool` (client): pre-established connections to reduce initial latency; aggressive mode intensifies management
- `nodelay`: enables TCP_NODELAY for latency (may slightly reduce effective throughput)
- `mux_session`, `mux_version`: SMUX parameters with safe defaults
- Sniffer: sorted JSON storage of per-port usage with human-readable formatting (KB/MB/GB)
- Colored Logger: level set via `log_level`

