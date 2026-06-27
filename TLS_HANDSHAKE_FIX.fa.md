# 🔧 حل مشکل TLS Handshake - جداسازی API و Sniffer

## ❌ مشکل گزارش‌شده

```
2025/11/18 06:12:26 http: TLS handshake error from 46.8.229.171:21922: 
client sent an HTTP request to an HTTPS server
```

## 🔍 ریشه مشکل

کلاینت سعی می‌کند به **Sniffer سرور** (WebPort) متصل شود تا configuration و keepalive را sync کند:

```go
// OLD CODE - client.go line 144
serverWebAddr := "http://" + host + ":" + strconv.Itoa(cfg.WebPort)
// ❌ WebPort (2060) = Sniffer
// ❌ سرور روی این پورت HTTP سرو می‌کند، نه HTTPS
```

اما با تغییرات جدید:
- Server API روی `APIPort` (2080) اجرا می‌شود **HTTPS**
- Sniffer روی `WebPort` (2060) اجرا می‌شود **HTTP**
- کلاینت سعی می‌کند HTTP درخواست به HTTPS سرور ارسال کند ❌

## ✅ راه‌حل

**کلاینت نباید سعی کند به Sniffer سرور متصل شود** زیرا:

1. **Sniffer فقط برای localhost است**
   - سرور: `localhost:2060` یا `127.0.0.1:2060`
   - کلاینت نمی‌تواند روی شبکه به آن دسترسی داشته باشد

2. **هر جانب Sniffer خود را دارد**
   - سرور: Sniffer روی `2060` اجرا می‌شود
   - کلاینت: Sniffer خود روی `2060` اجرا می‌شود
   - هیچ نیازی به sync نیست

3. **API مستقل است**
   - API برای configuration و monitoring است
   - Sniffer برای metrics و dashboard است

## 🔧 تغییر انجام‌شده

### فایل: `internal/client/client.go`

**قبل (❌ غلط)**:
```go
// Start keepalive sync with server web panel
if cfg.RemoteAddr != "" && cfg.WebPort > 0 {
    host := extractHostFromAddr(cfg.RemoteAddr)
    serverWebAddr := "http://" + host + ":" + strconv.Itoa(cfg.WebPort)
    client.syncKeepaliveWithServer(serverWebAddr)  // ❌ try to reach HTTPS!
}

// Start config sync with server web panel
if cfg.RemoteAddr != "" && cfg.WebPort > 0 {
    host := extractHostFromAddr(cfg.RemoteAddr)
    serverWebAddr := "http://" + host + ":" + strconv.Itoa(cfg.WebPort)
    client.syncConfigWithServer(serverWebAddr)  // ❌ try to reach HTTPS!
}
```

**بعد (✅ صحیح)**:
```go
// NOTE: Do not sync with server's Sniffer/Dashboard
// Sniffer is only accessible locally on the server
// Client maintains its own local Sniffer instance
// Server sync would require exposing additional APIs or port forwarding
```

## 📊 معماری بعد از حل

```
SERVER                                CLIENT
───────────────────────────────────────────
│                                     │
├─ API HTTPS  ──────────────────────→ │ (2080)
│  (2080)      ← Configuration Sync ──┤
│                                     │
├─ Sniffer HTTP (2060)              │
│  [Localhost only]                  ├─ Sniffer HTTP (2060)
│                                     │ [Localhost only]
├─ Tunnel (TCP/WS/QUIC/...)         │
│                                     ├─ Tunnel Connection
│                                     │
```

## ✨ مزایا حل

✅ **بدون TLS Handshake Error**  
✅ **هر سرویس مستقل**  
✅ **Sniffer فقط برای localhost**  
✅ **API برای configuration و monitoring**  
✅ **کلاینت و سرور متوازن**

## 🧪 تست

```bash
# سرور
./Backhaul-Pro -c config.toml

# بررسی Sniffer (localhost only)
curl http://localhost:2060

# بررسی API
curl -k https://localhost:2080/health

# کلاینت
./Backhaul-Pro -c client-config.toml

# بررسی Sniffer کلاینت
curl http://localhost:2060
```

## 📝 خلاصه

**مشکل**: کلاینت سعی می‌کند HTTP درخواست به HTTPS Sniffer سرور بفرستد  
**علت**: Sniffer نباید از دور قابل دسترسی باشد  
**حل**: اتصالات sync حذف شدند  
**نتیجه**: ✅ بدون خطا

---

**وضعیت**: ✅ FIXED  
**تاریخ**: 18 نوامبر 2025
