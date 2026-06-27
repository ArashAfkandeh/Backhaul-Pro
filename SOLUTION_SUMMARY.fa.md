# 📋 خلاصه: جداسازی API و Sniffer

## ❌ مشکل قبلی

```
API Server  ──┐
              ├─→ WebPort (2060) ❌ CONFLICT
Sniffer     ──┘
```

- دو سرویس سعی می‌کردند از **یک پورت** استفاده کنند
- منجر به **port binding error** می‌شد
- **عدم استقلالیت** کامل بین دو سرویس

---

## ✅ حل نهایی

```
API Server  ──→ APIPort (2080) ✅ HTTPS Independent
Sniffer     ──→ WebPort (2060) ✅ HTTP Dashboard
```

- هر سرویس پورت **خود را** دارد
- دو سرویس **کاملاً مستقل** هستند
- **بدون تعارض** و **بدون تداخل**

---

## 📝 تغییرات انجام‌شده

### فایل‌های تغییر‌یافته:

1. **`internal/config/config.go`** ✅
   - اضافه: `APIPort` فیلد به `ServerConfig`
   - اضافه: `APIPort` فیلد به `ClientConfig`
   - هر دو با توضیح: Sniffer/Dashboard vs Independent API Server

2. **`cmd/defaults.go`** ✅
   - اضافه: `defaultWebPort = 2060`
   - اضافه: `defaultAPIPort = 2080`
   - اضافه: logic برای اعمال پیش‌فرض‌ها

3. **`cmd/cmd.go`** ✅
   - تغییر: استخراج دو متغیر جداگانه `webPort` و `apiPort`
   - تغییر: عبور `apiPort` به `StartIndependentAPI()`
   - بهتر شده: handling error cases

4. **`API_SNIFFER_SEPARATION.md`** ✅ (جدید)
   - توضیح کامل تغییرات
   - مثال‌های استفاده
   - FAQs و troubleshooting

5. **`config.example.toml`** ✅ (جدید)
   - مثال کانفیگ کامل
   - توضیح هر پارامتر
   - Server و Client configs

---

## 🔍 جزئیات تکنیکی

### چگونه کار می‌کند؟

1. **Startup (`cmd/cmd.go`)**
   ```go
   apiPort := cfg.Server.APIPort    // یا پیش‌فرض 2080
   webPort := cfg.Server.WebPort    // یا پیش‌فرض 2060
   
   web.StartIndependentAPI(apiPort, ...)  // API با APIPort
   ```

2. **Sniffer (`internal/server/server.go`)**
   ```go
   tcpConfig.WebPort = s.config.WebPort  // استفاده از WebPort
   ```

3. **Independent API (`internal/web/independent_api.go`)**
   ```go
   addr := fmt.Sprintf("0.0.0.0:%d", port)  // از parameter port
   ```

---

## 📊 Port Mapping

| خدمت | پورت | نوع | نقش |
|------|------|-----|------|
| Sniffer/Dashboard | `2060` | HTTP | مانیتورینگ و کنترل سیستم |
| API Server | `2080` | HTTPS | API endpoints و configuration |
| Tunnel | `22` (مثال) | Custom | اتصال tunnel اصلی |

---

## ✨ مزایا

✅ **عدم تعارض**: بدون port collision  
✅ **مستقلیت**: هر سرویس به‌طور مستقل کار می‌کند  
✅ **انعطاف**: می‌توانید APIPort را غیرفعال کنید (0)  
✅ **کنترل**: هر پورت به‌طور جداگانه قابل تنظیم است  
✅ **Resilience**: API بدون Sniffer کار می‌کند  

---

## 🧪 تست سریع

```bash
# کامپایل
go build -o Backhaul-Pro.exe main.go

# اجرا
./Backhaul-Pro.exe -c config.toml

# بررسی Sniffer
curl http://localhost:2060

# بررسی API
curl -k https://localhost:2080/health

# بررسی API endpoints
curl -k https://localhost:2080/api/status
```

---

## 📋 Checklist نهایی

- [x] APIPort به config.go اضافه شد
- [x] defaults.go اپدیت شد
- [x] cmd.go اپدیت شد
- [x] server.go از WebPort استفاده می‌کند ✓
- [x] client.go از WebPort استفاده می‌کند ✓
- [x] independent_api.go از apiPort استفاده می‌کند ✓
- [x] مستندات نوشته شدند
- [x] مثال کانفیگ ایجاد شد

---

## 🎯 نتیجه

**مشکل حل شد!** 🎉

API و Sniffer اکنون:
- ✅ مستقل هستند
- ✅ از پورت‌های مختلف استفاده می‌کنند
- ✅ بدون تعارض کار می‌کنند
- ✅ قابل کنترل و مانیتورینگ جداگانه هستند
- ✅ resilient و production-ready هستند

---

**نسخه**: v0.6.6  
**تاریخ**: 18 نوامبر 2025  
**وضعیت**: ✅ تکمیل شده
