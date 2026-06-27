# API و Sniffer - جداسازی پورت‌ها

## 📋 خلاصه مسئله

قبلاً، سرویس **API مستقل** و **Sniffer/Dashboard** هر دو سعی می‌کردند از **یک پورت** (`WebPort`) استفاده کنند، که به تعارض منجر می‌شد.

## ✅ راه‌حل

دو پورت جداگانه تعریف کردیم:

1. **`WebPort`** (پیش‌فرض: `2060`) - برای Sniffer و Dashboard
2. **`APIPort`** (پیش‌فرض: `2080`) - برای سرویس API مستقل

## 🔧 تغییرات انجام‌شده

### 1. **`internal/config/config.go`**

اضافه کردن فیلد `APIPort` به ساختارهای کانفیگ:

```go
// ServerConfig
type ServerConfig struct {
    // ... existing fields ...
    WebPort          int    `toml:"web_port"`       // Sniffer/Dashboard port
    APIPort          int    `toml:"api_port"`       // Independent API server port
    SnifferLog       string `toml:"sniffer_log"`
    // ...
}

// ClientConfig  
type ClientConfig struct {
    // ... existing fields ...
    WebPort          int    `toml:"web_port"`       // Sniffer/Dashboard port
    APIPort          int    `toml:"api_port"`       // Independent API server port
    SnifferLog       string `toml:"sniffer_log"`
    // ...
}
```

### 2. **`cmd/defaults.go`**

اضافه کردن پیش‌فرض‌ها برای دو پورت:

```go
const (
    // ... existing constants ...
    defaultWebPort  = 2060 // Sniffer/Dashboard port
    defaultAPIPort  = 2080 // Independent API server port
)

func applyDefaults(cfg *config.Config) {
    // ... existing code ...
    
    // WebPort (Sniffer/Dashboard)
    if cfg.Server.WebPort <= 0 {
        cfg.Server.WebPort = defaultWebPort
    }
    if cfg.Client.WebPort <= 0 {
        cfg.Client.WebPort = defaultWebPort
    }
    
    // APIPort (Independent API server)
    if cfg.Server.APIPort <= 0 {
        cfg.Server.APIPort = defaultAPIPort
    }
    if cfg.Client.APIPort <= 0 {
        cfg.Client.APIPort = defaultAPIPort
    }
}
```

### 3. **`cmd/cmd.go`**

تغییر استخراج پورت برای استفاده از دو متغیر جداگانه:

```go
func Run(configPath string, ctx context.Context) *config.Config {
    var webPort int = 2060   // Sniffer/Dashboard
    var apiPort int = 2080   // API server

    // ... load config ...
    
    // Extract ports from valid config
    if cfg.Server != nil {
        if cfg.Server.WebPort > 0 {
            webPort = cfg.Server.WebPort
        }
        if cfg.Server.APIPort > 0 {
            apiPort = cfg.Server.APIPort
        }
    }
    
    // Start independent API server with APIPort
    logger.Printf("INFO: Starting independent API server (port %d)", apiPort)
    web.StartIndependentAPI(apiPort, logger, configPath, ctx, cancel)
}
```

### 4. **`internal/server/server.go`** (بدون تغییر)

تمام Transport Config‌ها از `WebPort` استفاده می‌کنند برای Sniffer:

```go
// TCP
tcpConfig := &transport.TcpConfig{
    // ...
    WebPort:    s.config.WebPort,  // ✅ Sniffer/Dashboard
    // ...
}

// TCPMUX
tcpMuxConfig := &transport.TcpMuxConfig{
    // ...
    WebPort:    s.config.WebPort,  // ✅ Sniffer/Dashboard
    // ...
}

// WebSocket
wsConfig := &transport.WsConfig{
    // ...
    WebPort:    s.config.WebPort,  // ✅ Sniffer/Dashboard
    // ...
}

// ... similar for all other transports ...
```

### 5. **`internal/client/client.go`** (بدون تغییر)

تمام Transport Config‌ها از `WebPort` استفاده می‌کنند برای Sniffer:

```go
// TCP
tcpConfig := &transport.TcpConfig{
    WebPort:    c.config.WebPort,  // ✅ Sniffer/Dashboard
}

// WebSocket
WsConfig := &transport.WsConfig{
    WebPort:    c.config.WebPort,  // ✅ Sniffer/Dashboard
}

// QUIC
quicConfig := &transport.QuicConfig{
    SnifferPort: c.config.WebPort, // ✅ Sniffer/Dashboard
}

// ... similar for all other transports ...
```

### 6. **`internal/web/independent_api.go`** (بدون تغییر)

سرویس API مستقل قبلاً تنها از پارامتری که قبول می‌کند استفاده می‌کند:

```go
func StartIndependentAPI(port int, logger *logrus.Logger, configPath string, ctx context.Context, cancel context.CancelFunc) {
    // ... 
    addr := fmt.Sprintf("0.0.0.0:%d", port)  // ✅ APIPort
    // ...
}
```

## 📝 تغییر در فایل کانفیگ TOML

قبل (اشتراک پورت):
```toml
[server]
# فقط یک پورت
web_port = 2060
```

بعد (جداسازی پورت‌ها):
```toml
[server]
web_port = 2060   # Sniffer/Dashboard
api_port = 2080   # Independent API server

[client]
web_port = 2060   # Sniffer/Dashboard
api_port = 2080   # Independent API server
```

## 🎯 فایده‌های این جداسازی

✅ **مستقلیت کامل**: API و Sniffer دیگر از هم تداخل ندارند  
✅ **قابلیت کنترل**: هر سرویس پورت خود را داراست  
✅ **سهولت مانیتورینگ**: می‌توانید هر دو را به‌طور جداگانه مانیتور کنید  
✅ **انعطاف پذیری**: می‌توانید API را بدون Sniffer اجرا کنید یا برعکس  
✅ **عدم تعارض**: هیچ port collision یا binding error وجود ندارد  

## 🔍 نحوه استفاده

### Server
```toml
[server]
bind_addr = "0.0.0.0:22"
transport = "tcp"
web_port = 2060   # Sniffer at http://localhost:2060
api_port = 2080   # API at https://localhost:2080
```

### Client
```toml
[client]
remote_addr = "server.example.com:22"
transport = "tcp"
web_port = 2060   # Sniffer at http://localhost:2060
api_port = 2080   # API at https://localhost:2080
```

## 🧪 تست

```bash
# کامپایل
go build -o Backhaul-Pro.exe main.go

# اجرا با فایل کانفیگ
./Backhaul-Pro.exe -c config.toml

# بررسی Sniffer
curl http://localhost:2060

# بررسی API
curl -k https://localhost:2080/health
```

## 📊 خلاصه تغییرات

| فایل | تغییر | توضیح |
|------|------|-------|
| `internal/config/config.go` | ✅ اضافه | `APIPort` فیلد |
| `cmd/defaults.go` | ✅ اضافه | `defaultAPIPort` و logical |
| `cmd/cmd.go` | ✅ تغییر | استخراج `apiPort` جداگانه |
| `internal/server/server.go` | ✔️ تغییر نکرد | از `WebPort` برای Sniffer |
| `internal/client/client.go` | ✔️ تغییر نکرد | از `WebPort` برای Sniffer |
| `internal/web/independent_api.go` | ✔️ تغییر نکرد | از parameter `port` |

---

## ℹ️ نکات مهم

- `WebPort` **تنها** برای Sniffer/Dashboard است
- `APIPort` **تنها** برای سرویس API مستقل است
- دو پورت باید **متفاوت** باشند
- پیش‌فرض‌ها ایمن‌ترین مقادیر هستند
- API **مستقل و resilient** باقی می‌ماند

---

**نتیجه**: API و Sniffer اکنون مستقل‌تر هستند و هیچ port collision وجود ندارد! 🎉
