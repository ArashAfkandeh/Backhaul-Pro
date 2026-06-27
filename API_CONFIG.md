# Config Update API Documentation

## نقطه پایانی (Endpoint)

```
POST/PUT http://localhost:<web_port>/api/config
```

## توضیح

این API امکان تغییر پویا (Runtime) تنظیمات سرور و کلاینت را فراهم می‌کند **بدون نیاز به راه‌اندازی مجدد برنامه**.

## 🔐 امنیت (Authentication)

تمام درخواست‌ها **باید** شامل `token` موجود در فایل کانفیگ باشند. بدون token درخواست رد می‌شود.

---

## درخواست (Request)

### Headers
```
Content-Type: application/json
```

### Body Structure

```json
{
  "type": "server",
  "token": "YOUR_SECRET_TOKEN_FROM_CONFIG",
  "log_level": "debug",
  "nodelay": true,
  "keepalive_period": 30,
  "pprof": false,
  "mux_session": 2,
  "mux_version": 2,
  "mux_framesize": 32768,
  "mux_recievebuffer": 67108864,
  "mux_streambuffer": 16777216,
  "sniffer": true,
  "heartbeat": 30,
  "mux_con": 1,
  "accept_udp": false,
  "channel_size": 2048,
  "ports": ["8080", "8081", "8082"],
  "tls_cert": "/path/to/cert.pem",
  "tls_key": "/path/to/key.pem"
}
```

---

## پارامترهای تنظیمات سرور (Server Config Parameters)

| پارامتر | نوع | توضیح | محدودیت |
|---------|------|---------|---------|
| `type` | string | `"server"` | الزامی |
| `token` | string | Token موجود در کانفیگ | الزامی |
| `log_level` | string | لاگ لول: trace, debug, info, warn, error, fatal | اختیاری |
| `nodelay` | boolean | TCP_NODELAY فعال/غیرفعال | اختیاری |
| `keepalive_period` | integer | مدت زمان Keep-Alive (ثانیه) | ≥ 0 |
| `pprof` | boolean | فعال کردن pprof debugging | اختیاری |
| `mux_session` | integer | تعداد نشست‌های Multiplexing | ≥ 1 |
| `mux_version` | integer | نسخه Multiplexing | 1 یا 2 |
| `mux_framesize` | integer | حجم فریم (بایت) | ≥ 1024 |
| `mux_recievebuffer` | integer | بافر دریافت (بایت) | - |
| `mux_streambuffer` | integer | بافر جریان (بایت) | - |
| `sniffer` | boolean | فعال/غیرفعال سنیفر ترافیک | اختیاری |
| `heartbeat` | integer | فاصله Heartbeat (ثانیه) | ≥ 1 |
| `mux_con` | integer | تعداد اتصالات Multiplexing | - |

> **نکته:** اگر چند ورودی مشابه برای یک پورت محلی تعیین شود، مقصدها به صورت لیست ذخیره می‌شوند و برنامه یک **الگوریتم بارگذاری ترکیبی** اجرا می‌کند: نخستین اتصال هر آدرس مبدا به مقصدی که تاکنون کم‌ترین اختصاص را داشته هدایت می‌شود، و پس از آن همان مبدا همیشه به همان مقصد می‌رسد. این رویکرد هم توزیع اولیه یکنواخت‌تری دارد هم به ترافیک حالت sticky می‌بخشد.
| `accept_udp` | boolean | پذیرش UDP | اختیاری |
| `channel_size` | integer | حجم کانال | ≥ 1 |
| `ports` | array | لیست پورت‌ها (پورت محلی یا نگاشت به آدرس/پورت‌ دیگر) – قابل تکرار برای پیکربندی لودبالانس | آپشن |
| `tls_cert` | string | مسیر فایل گواهی TLS | - |
| `tls_key` | string | مسیر فایل کلید TLS | - |

---

## پارامترهای تنظیمات کلاینت (Client Config Parameters)

| پارامتر | نوع | توضیح | محدودیت |
|---------|------|---------|---------|
| `type` | string | `"client"` | الزامی |
| `log_level` | string | لاگ لول | اختیاری |
| `nodelay` | boolean | TCP_NODELAY | اختیاری |
| `keepalive_period` | integer | مدت زمان Keep-Alive (ثانیه) | ≥ 0 |
| `pprof` | boolean | pprof debugging | اختیاری |
| `mux_session` | integer | نشست‌های Multiplexing | ≥ 1 |
| `mux_version` | integer | نسخه Multiplexing | 1 یا 2 |
| `mux_framesize` | integer | حجم فریم | ≥ 1024 |
| `mux_recievebuffer` | integer | بافر دریافت | - |
| `mux_streambuffer` | integer | بافر جریان | - |
| `sniffer` | boolean | سنیفر ترافیک | اختیاری |
| `retry_interval` | integer | فاصله تلاش دوباره | - |
| `dial_timeout` | integer | زمان تایم‌اوت اتصال | - |
| `aggressive_pool` | boolean | استفاده از aggressive pool | اختیاری |
| `channel_size` | integer | حجم کانال | ≥ 1 |
| `connection_pool` | integer | حجم Connection Pool | - |

---

## مثال‌های درخواست

### 1. تغییر Log Level سرور

```bash
curl -X POST http://localhost:2060/api/config \
  -H "Content-Type: application/json" \
  -d '{
    "type": "server",
    "token": "your-secret-token",
    "log_level": "debug"
  }'
```

### 2. فعال کردن Sniffer و تنظیم Heartbeat

```bash
curl -X POST http://localhost:2060/api/config \
  -H "Content-Type: application/json" \
  -d '{
    "type": "server",
    "token": "your-secret-token",
    "sniffer": true,
    "heartbeat": 30
  }'
```

### 3. تغییر تنظیمات Multiplexing

```bash
curl -X PUT http://localhost:2060/api/config \
  -H "Content-Type: application/json" \
  -d '{
    "type": "server",
    "token": "your-secret-token",
    "mux_version": 2,
    "mux_framesize": 65536,
    "mux_session": 2
  }'
```

### 4. تغییر تنظیمات کلاینت

```bash
curl -X POST http://localhost:2060/api/config \
  -H "Content-Type: application/json" \
  -d '{
    "type": "client",
    "token": "client-secret-token",
    "log_level": "info",
    "dial_timeout": 10,
    "aggressive_pool": true
  }'
```

### 5. بدون Token (خطا)

```bash
curl -X POST http://localhost:2060/api/config \
  -H "Content-Type: application/json" \
  -d '{
    "type": "server",
    "log_level": "debug"
  }'
```

**نتیجه**: `401 Unauthorized`

```json
{
  "success": false,
  "message": "Authentication token is required"
}
```

### 6. Token نادرست (خطا)

```bash
curl -X POST http://localhost:2060/api/config \
  -H "Content-Type: application/json" \
  -d '{
    "type": "server",
    "token": "wrong-token",
    "log_level": "debug"
  }'
```

**نتیجه**: `401 Unauthorized`

```json
{
  "success": false,
  "message": "Invalid authentication token"
}
```

---

---

## Health Score API

### نقطه پایانی (Endpoint)

```
GET http://localhost:<web_port>/api/health-score
```

### توضیح

این API نمره سلامت سیستم را برمی‌گرداند که شامل دو متریک **Hybrid** است:
- **ResourceScore**: امتیازی از 1 تا 100 بر اساس میانگین مصرف منابع (سرور + کلاینت)
- **NetworkScore**: امتیازی از 1 تا 100 بر اساس میانگین کیفیت شبکه (سرور + کلاینت)

**روش محاسبه:**
- نمرات سرور = بر اساس CPU، RAM، Latency، PacketLoss، Throughput سرور
- نمرات کلاینت = بر اساس متریک‌های فرستاده شده از کلاینت توسط Tuner
- نمره Hybrid = میانگین نمرات سرور و کلاینت
- اگر کلاینت متصل نباشد، فقط نمرات سرور استفاده می‌شود

### درخواست

```bash
curl http://localhost:2060/api/health-score
```

یا برای HTTPS:

```bash
curl --insecure https://localhost:2060/api/health-score
```

### پاسخ موفق (200 OK)

```json
{
  "resource_score": 75,
  "network_score": 82,
  "timestamp": 1731850000
}
```

### توضیح پاسخ

| فیلد | نوع | توضیح |
|------|------|--------|
| `resource_score` | integer | نمره Hybrid منابع سیستم (1-100) |
| `network_score` | integer | نمره Hybrid کیفیت شبکه (1-100) |
| `timestamp` | integer | Unix timestamp زمان دریافت |

### مثال Monitoring

```bash
# هر 30 ثانیه بررسی کنید
watch -n 30 'curl --insecure https://localhost:2060/api/health-score | jq .'
```

---

## Status API

### نقطه پایانی (Endpoint)

```
GET http://localhost:<web_port>/api/status
```

### توضیح

این API وضعیت فعلی برنامه را برمی‌گرداند. بدون نیاز به token.

### درخواست

```bash
curl https://localhost:2060/api/status
```

### پاسخ موفق (200 OK)

```json
{
  "success": true,
  "status": "running",
  "message": "Backhaul-Pro application is running"
}
```

---

## Restart API

### نقطه پایانی (Endpoint)

```
POST http://localhost:<web_port>/api/restart
```

### توضیح

این API برنامه را به‌صورت امن و بدون قطع سرویس راه‌اندازی مجدد می‌کند. **نیاز به token دارد**.

### درخواست

```bash
curl -X POST https://localhost:2060/api/restart \
  -H "Content-Type: application/json" \
  -d '{
    "token": "YOUR_SECRET_TOKEN_FROM_CONFIG",
    "type": "server"
  }'
```

### پاسخ موفق (200 OK)

```json
{
  "success": true,
  "message": "Restart signal sent, application will restart shortly"
}
```

### پاسخ خطا - Token مفقود (401 Unauthorized)

```json
{
  "success": false,
  "message": "Authentication token is required"
}
```

### پاسخ خطا - Token نادرست (401 Unauthorized)

```json
{
  "success": false,
  "message": "Invalid authentication token"
}
```

---
GET http://localhost:<web_port>/api/status
```

### توضیح

این API وضعیت فعلی برنامه را برمی‌گرداند.

### درخواست

```bash
curl http://localhost:2060/api/status
```

### پاسخ (200 OK)

```json
{
  "success": true,
  "status": "running",
  "message": "Backhaul-Pro application is running"
}
```

---

## Restart API

### نقطه پایانی (Endpoint)

```
POST http://localhost:<web_port>/api/restart
```

### توضیح

این API برنامه را به صورت Hot Reload راه‌اندازی مجدد می‌کند (بدون قطع سرویس).

### درخواست

```bash
curl -X POST http://localhost:2060/api/restart
```

### پاسخ (200 OK)

```json
{
  "success": true,
  "message": "Restart signal sent, application will restart shortly"
}
```

---

### موفق (200 OK)

```json
{
  "success": true,
  "message": "Server configuration updated successfully",
  "data": {
    "appliedChanges": {
      "log_level": {
        "old": "info",
        "new": "debug"
      },
      "sniffer": {
        "old": false,
        "new": true
      }
    }
  }
}
```

### خطای Validation (400 Bad Request)

```json
{
  "success": false,
  "message": "Validation errors",
  "data": [
    {
      "field": "heartbeat",
      "message": "heartbeat must be at least 1 second"
    },
    {
      "field": "mux_version",
      "message": "mux_version must be 1 or 2"
    }
  ]
}
```

### خطای Parsing JSON (400 Bad Request)

```json
{
  "success": false,
  "message": "Invalid JSON request: unexpected end of JSON input"
}
```

### خطای Config Not Found (404)

```json
{
  "success": false,
  "message": "Server config not found"
}
```

### خطای Method Not Allowed (405)

```json
{
  "success": false,
  "message": "Only POST and PUT methods are supported"
}
```

---

## ویژگی‌های API

✅ **Validation سختگیرانه**: تمام پارامترها قبل از اعمال تصدیق می‌شوند

✅ **Thread-Safe**: استفاده از Mutex برای تغییرات امن

✅ **تغییر فقط تفاوت‌ها**: فقط فیلدهایی که تغییر کرده‌اند اعمال می‌شوند

✅ **Detailed Logging**: تمام تغییرات در لاگ ثبت می‌شوند

✅ **Change Tracking**: پاسخ شامل مقادیر قبل و بعد است

---

## نکات مهم

⚠️ **تغییر Port‌ها**: برای اعمال تغییر port، برنامه باید مجدداً راه‌اندازی شود

⚠️ **TLS تغییرات**: تغییر فایل‌های TLS نیاز به راه‌اندازی مجدد دارد

⚠️ **Transport Type**: نمی‌توان Transport را از طریق این API تغییر داد

ℹ️ **بدون ریستارت**: تمام تغییرات **بدون نیاز به راه‌اندازی مجدد** اعمال می‌شوند

---

## مثال Python

```python
import requests
import json

url = "http://localhost:2060/api/config"
payload = {
    "type": "server",
    "log_level": "debug",
    "sniffer": True,
    "heartbeat": 30
}

response = requests.post(url, json=payload)
print(json.dumps(response.json(), indent=2))
```

## مثال JavaScript

```javascript
const config = {
  type: "server",
  log_level: "debug",
  sniffer: true,
  heartbeat: 30
};

fetch('http://localhost:2060/api/config', {
  method: 'POST',
  headers: {
    'Content-Type': 'application/json',
  },
  body: JSON.stringify(config)
})
.then(response => response.json())
.then(data => console.log(data));
```

---

## Status Code‌ها

| Code | معنی |
|------|------|
| 200 | درخواست موفق و Config به‌روز شد |
| 400 | خطای Validation یا JSON نامعتبر |
| 401 | Token مفقود یا نادرست (Unauthorized) |
| 404 | Config یافت نشد |
| 405 | Method غیر پشتیبانی (فقط POST/PUT) |
| 500 | خطای سرور (Config Provider تنظیم نشده) |

---

## 🔐 نکات امنیتی

✅ **Token الزامی**: تمام درخواست‌ها نیاز به token دارند

✅ **Token مقابله‌ای**: Token از کانفیگ فایل می‌آید

✅ **Logging**: تلاش‌های ناموفق در لاگ ثبت می‌شوند

✅ **HTTP Status**: استفاده از `401 Unauthorized` برای token نادرست

⚠️ **HTTPS توصیه می‌شود**: در Production از HTTPS استفاده کنید
