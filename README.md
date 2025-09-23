## بک‌هاول حرفه‌ای

یک سیستم تونل معکوس با کارایی بالا برای عبور از NAT و فایروال، با پشتیبانی از TCP/WS/WSS و حالت‌های مولتی‌پلکس، به‌همراه وب‌پنل نظارتی، Sniffer اختیاری و تیونینگ خودکار پارامترها.

- نسخه برنامه: v0.6.6
- زبان: Go 1.23+
- مخزن گیت‌هاب: [Backhaul-Pro](https://github.com/ArashAfkandeh/Backhaul-Pro.git)
- لایسنس: AGPL-3.0

### فهرست مطالب
- معرفی و معماری
- قابلیت‌ها و مزایا
- ترنسپورت‌ها و سناریوهای استفاده
- امنیت و احراز هویت
- وب‌پنل و API‌های نظارتی
- تیونینگ خودکار (Auto-Tune)
- Hot Reload پیکربندی
- نصب و به‌روزرسانی (با استفاده از installer.sh)
- نصب دستی از سورس
- اجرای سرویس (systemd) و مدیریت سرویس‌ها
- نمونه پیکربندی سرور و کلاینت با تیونینگ فعال
- نکات عملیاتی و عیب‌یابی
- ساختار کد و فایل‌های مهم
- لایسنس و حمایت

---

### معرفی و معماری
Backhaul برای عبور امن و مقیاس‌پذیر ترافیک از پشت NAT/فایروال طراحی شده است. این سیستم از یک باینری واحد تشکیل شده که بسته به محتوای `config.toml` در نقش سرور یا کلاینت اجرا می‌شود.

- فرایند اصلی (`main.go`):
  - پردازش فلگ‌ها (`-c`, `-v`, `--no-auto-tune`, `--tune-interval`)
  - اعمال تیونینگ موقت TCP سیستم‌عامل (startup)
  - راه‌اندازی عملیات اصلی از طریق `cmd.Run`
  - راه‌اندازی تیونر پویا (در صورت فعال بودن)
  - Hot Reload با مانیتور تغییر فایل کانفیگ
  - مدیریت خاموشی امن و Force Shutdown

- ماژول فرمان (`cmd/`):
  - بارگذاری TOML، اعمال پیش‌فرض‌ها (`cmd/defaults.go`)
  - انتخاب نقش: سرور (`internal/server`) یا کلاینت (`internal/client`)

- پشته انتقال‌ها (Transport):
  - TCP, TCPMUX, WS/WSS, WSMUX/WSSMUX, UDP
  - مولتی‌پلکسینگ با SMUX (کانفیگ پارامترهای `mux_*`)

- وب‌پنل و Sniffer (`internal/web/`):
  - داشبورد HTML داخلی (Tailwind)
  - Endpointهای `/`, `/stats`, `/data`, `/config`
  - ذخیره و ارائه گزارش‌های مصرف پورت‌ها در JSON

---

### قابلیت‌ها و مزایا
- **کارایی بالا**: پیاده‌سازی بهینه برای تعداد زیاد اتصال همزمان.
- **ترنسپورت‌های متنوع**: `tcp`, `tcpmux`, `ws`, `wss`, `wsmux`, `wssmux`, `udp`.
- **مولتی‌پلکس (SMUX)**: تجمیع چند اتصال در یک تونل با کنترل جریان.
- **وب‌پنل مانیتورینگ**: آمار لحظه‌ای سیستم و تونل + مشاهده کانفیگ جاری.
- **Sniffer اختیاری**: ثبت مصرف پورت‌ها در فایل JSON با تبدیل خوانا.
- **تیونینگ خودکار**: تنظیم پویای `keepalive`, `mux_*`, `channel_size`, `connection_pool`, `heartbeat`, `mux_con`.
- **Hot Reload**: تغییرات کانفیگ به‌صورت امن اعمال می‌شوند.
- **PPROF**: پروفایلینگ اختیاری روی پورت‌های 6060/6061 (Server/Client).

توجه: در این پروژه، پشتیبانی از QUIC حذف شده است. تمرکز روی TCP/WS/WSS (+MUX) و UDP است.

---

### ترنسپورت‌ها و سناریوهای استفاده
- **TCP**: ساده و سریع؛ مناسب شبکه‌های پایدار.
- **TCPMUX**: کاهش تعداد کانکشن‌های فیزیکی با چندجریانی در یک اتصال.
- **WS/WSS**: عبور از پروکسی/فایروال‌های لایه HTTP؛ در WSS با TLS امن می‌شود.
- **WSMUX/WSSMUX**: ترکیب مزایای WS/WSS با مولتی‌پلکس.
- **UDP**: تونل‌کردن UDP (و یا عبور UDP روی TCP سمت سرور با `accept_udp`).

سناریوها:
- بای‌پس محدودیت‌ها با `wss` روی پورت‌های عمومی 443/8443.
- تراکم اتصال بالا با `tcpmux` یا `wsmux/wssmux` و تنظیم `mux_*`.
- محیط‌های محدود TCP با `ws/wss` و گزینه‌ی `edge_ip` (CDN-Friendly).

---

### امنیت و احراز هویت
- **توکن**: تمامی درخواست‌های تونل با `token` احراز می‌شوند. مقدار قوی و تصادفی تعریف کنید.
- **TLS** (فقط WSS/WSSMUX): از گواهی معتبر استفاده کنید. نمونه ساخت گواهی خودامضاء در انتهای فایل آمده است.
- **وب‌پنل**: دسترسی را محدود کنید (IP Whitelist/Firewall/Reverse Proxy) یا روی اینترفیس لوکال ارائه دهید.

---

### وب‌پنل و API‌های نظارتی
با مقداردهی `web_port > 0` فعال می‌شود.
- `/` داشبورد HTML برای مشاهده کانفیگ، وضعیت تونل، و آمار سیستم
- `/stats` خروجی JSON: CPU/RAM/Disk/Swap/Traffic/BackhaulTraffic/Connections/Status
- `/data` خروجی JSON از مصرف پورت‌ها (فقط وقتی `sniffer=true`)
- `/config` کانفیگ جاری بدون فیلدهای حساس؛ `?type=client` برای دریافت کانفیگ کلاینت

همگام‌سازی پویا (سمت کلاینت):
- کلاینت به‌صورت دوره‌ای برخی پارامترها (مانند `keepalive_period` و `mux_*`) را با `/config` سرور همگام می‌کند.

---

### تیونینگ خودکار (Auto-Tune)
به‌صورت پیش‌فرض فعال است و می‌توان با `--no-auto-tune` غیرفعال کرد. بازه اجرای تیونینگ با `--tune-interval` (پیش‌فرض 10m) قابل تنظیم است.
- ورودی‌های تصمیم: CPU، RAM، Latency (TCP Dial)، Packet Loss، Throughput
- خروجی‌های تنظیم:
  - سمت کلاینت: `connection_pool`
  - سمت سرور: `channel_size`, `mux_*` (framesize/receivebuffer/streambuffer), `heartbeat`, `mux_con`
  - هر دو: `keepalive_period` و همگام‌سازی آن‌ها
- تنظیم پویا و تغییر بازه تیونینگ بر اساس پایداری شبکه (Variance/Avg)

---

### Hot Reload پیکربندی
فایل مشخص‌شده با `-c` پایش می‌شود؛ با تغییر زمان ویرایش:
- توقف امن instance قبلی (cancel context) و ایجاد instance جدید
- توقف/راه‌اندازی مجدد Tuner در صورت فعال بودن

---

### نصب و به‌روزرسانی (با استفاده از installer.sh)
اسکریپت نصب تعاملی، نصب آنلاین/آفلاین، ایجاد سرویس systemd، ساخت یا ویرایش فایل کانفیگ و مدیریت متمرکز را خودکار می‌کند.

- دانلود/اجرای اسکریپت (روی Debian/Ubuntu با sudo):
```bash
curl -LO https://raw.githubusercontent.com/ArashAfkandeh/Backhaul-Pro/main/installer.sh
sudo bash installer.sh
```

- اجرای مستقیم بدون ذخیره فایل:
```bash
bash <(curl -fsSL https://raw.githubusercontent.com/ArashAfkandeh/Backhaul-Pro/main/installer.sh)
```

- اجرای مستقیم با آرگومان‌ها (بدون ذخیره فایل):
# Uninstall all or selected services via interactive menu
```bash
bash <(curl -fsSL https://raw.githubusercontent.com/ArashAfkandeh/Backhaul-Pro/main/installer.sh) uninstall
```

# Open central management menu (status/logs/restart/edit/uninstall)
```bash
bash <(curl -fsSL https://raw.githubusercontent.com/ArashAfkandeh/Backhaul-Pro/main/installer.sh) manage
```

- حالت نصب:
  - Online: نصب وابستگی‌ها با apt، دانلود بسته باینری و استخراج به `/root/backhaul_pro`
  - Offline: نصب از آرشیو محلی `/root/backhaul_pro.tar.gz` و مخزن آفلاین بسته‌ها

- مراحل کلیدی:
  1) آماده‌سازی سیستم و وابستگی‌ها (wget, curl, openssl, tar, net-tools)
  2) ایجاد پوشه `/root/backhaul_pro`
  3) استخراج بسته و کپی باینری `backhaul_pro`
  4) انتخاب نقش (Server/Client)، پورت تونل، پروتکل (udp/tcp/tcpmux/ws/wss/wsmux/wssmux)، تولید/ورود Token، تعیین `web_port`
  5) (برای سرور) تعریف `ports` با اعتبارسنجی و چک اشغال‌بودن پورت‌های محلی
  6) تولید فایل `config.toml` مناسب نقش انتخابی
  7) ایجاد سرویس systemd مانند `backhaul_pro.service` (یا شماره‌دار)
  8) فعال‌سازی و راه‌اندازی سرویس، نمایش وضعیت
  9) نصب ابزار مدیریت مرکزی `bh-p` در `/usr/local/bin`

- مدیریت سرویس‌ها با `bh-p`:
  - لیست سرویس‌ها، وضعیت/لاگ زنده، ری‌استارت، ویرایش کانفیگ، نمایش اطلاعات اتصال (Server)، حذف سرویس

- حذف سرویس‌ها:
```bash
sudo bash installer.sh uninstall
# یا ابزار مدیریت مرکزی
sudo bh-p   # گزینه‌های Uninstall در منو
```

یادداشت: اگر `config.toml` موجود باشد، اسکریپت امکان جایگزینی/ایجاد فایل شماره‌دار جدید و مدیریت سرویس متناظر را می‌دهد.

---

### نصب دستی از سورس
```bash
git clone https://github.com/ArashAfkandeh/Backhaul-Pro.git
cd Backhaul-Pro
go build -o backhaul_pro
./backhaul_pro -c /path/to/config.toml
```
- نمایش نسخه: `./backhaul_pro -v`
- غیرفعال‌سازی تیونینگ خودکار: `--no-auto-tune`
- تغییر بازه تیونینگ: `--tune-interval 15m`

---

### اجرای سرویس (systemd) و مدیریت سرویس‌ها
اسکریپت نصب فایل سرویس را ایجاد می‌کند. نمونه سرویس (اگر نیاز به ساخت دستی داشتید):
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
دستورات رایج:
```bash
sudo systemctl daemon-reload
sudo systemctl enable backhaul_pro.service
sudo systemctl start backhaul_pro.service
sudo systemctl status backhaul_pro.service
sudo journalctl -u backhaul_pro.service -f
```

---

### نمونه پیکربندی سرور و کلاینت با تیونینگ فعال
تیونینگ خودکار به‌صورت پیش‌فرض فعال و بر روی مقدار 10 دقیقه است؛ کافیست بدون `--no-auto-tune` اجرا کنید. در مثال‌ها وب‌پنل برای مانیتورینگ روشن است.

- سرور (مثال WSSMUX روی 443 با مولتی‌پلکس و Sniffer فعال):
```toml
[server]
bind_addr = "0.0.0.0:443"
transport = "wssmux"
token = "YOUR_TOKEN"
web_port = 2060
# نگاشت پورت‌ها
ports = [
  "443-600",
  "443-600:5201",
  "443-600=1.1.1.1:5201",
]
```
اجرای سرور با تیونینگ فعال (بازه 10 دقیقه):
```bash
./backhaul_pro -c /root/backhaul_pro/config.toml --tune-interval 10m
```

- کلاینت (مثال WSSMUX با همگام‌سازی از وب‌پنل سرور):
```toml
[client]
remote_addr = "SERVER_IP:443"
transport = "wssmux"
token = "YOUR_TOKEN"
web_port = 2061
# اختیاری برای سناریوهای CDN در WS/WSS
edge_ip = ""
```
اجرای کلاینت با تیونینگ فعال:
```bash
./backhaul_pro -c /root/backhaul_pro/config.toml --tune-interval 10m
```

- نکته: تیونینگ در هر دو سمت فعال است و Keepalive بین سرور/کلاینت همگام می‌شود. برای غیرفعال‌سازی: `--no-auto-tune`.

---

### نکات عملیاتی و عیب‌یابی
- Token قوی تعریف و به‌صورت امن توزیع کنید.
- دسترسی به وب‌پنل را محدود کنید یا فقط روی لوکال ارائه دهید.
- روی پورت‌های عمومی (443) از WSS/WSSMUX با گواهی معتبر استفاده کنید.
- اگر کانفیگ تغییر نکرد اجرا نمی‌شود: مسیر `-c` را بررسی کنید. لاگ‌ها را ببینید.
- برای تاخیر بالا/نوسان شبکه، از MUX و تنظیم `mux_*` استفاده و به تیونینگ فرصت تطبیق بدهید.
- اگر پورت اشغال است، در اسکریپت نصب پیام خطا دریافت می‌کنید؛ پورت دیگری را انتخاب کنید.

---

### ساخت گواهی خودامضاء (فقط برای تست WSS/WSSMUX)
```bash
openssl genpkey -algorithm RSA -out server.key -pkeyopt rsa_keygen_bits:2048
openssl req -new -key server.key -out server.csr
openssl x509 -req -in server.csr -signkey server.key -out server.crt -days 365
```

---

### ساختار کد و فایل‌های مهم
- `main.go`: اجرای برنامه، سیگنال‌ها، Hot Reload، تیونر
- `cmd/`: بارگذاری/اعتبارسنجی کانفیگ، اعمال پیش‌فرض‌ها، انتخاب نقش اجرا
- `internal/config/`: انواع و ساختار کانفیگ‌ها
- `internal/server`, `internal/client`: راه‌اندازی ترنسپورت‌ها بر اساس نوع انتخابی
- `internal/*/transport`: پیاده‌سازی‌های `tcp`, `tcpmux`, `ws/wss`, `wsmux/wssmux`, `udp`
- `internal/web/`: داشبورد و API‌ها (`/`, `/stats`, `/data`, `/config`)
- `internal/tuning/`: منطق تیونینگ پویا و همگام‌سازی پارامترها
- `internal/utils/logger.go`: Logger سفارشی رنگی

---

### لایسنس و حمایت
این پروژه تحت مجوز AGPL-3.0 منتشر شده است. به فایل `LICENSE` مراجعه کنید.

از همراهی شما سپاسگزاریم.

---

### ابهام‌سازی ترافیک (Obfuscation)
برای عبور از DPI و فیلترینگ، در حالت‌های مبتنی بر وب‌سوکت (WS/WSS/W(S)MUX) لایه‌های ابهام‌سازی در سطح HTTP/TLS/مسیر اعمال می‌شود تا ترافیک تا حد ممکن شبیه ترافیک واقعی مرورگر باشد:
- هدرهای HTTP واقع‌نما (Accept, Accept-Language, Accept-Encoding, DNT, Connection و…)
- چرخش User-Agent بین مقادیر رایج (Chrome/Firefox/Safari/Edge روی پلتفرم‌های مختلف)
- مسیرهای وب‌سوکت شبه‌واقعی و پویا مانند `/api/v1/stream`, `/cdn/assets`, `/ws/chat`, …
- تنظیمات TLS شبیه مرورگرهای مدرن (TLS 1.2/1.3، منحنی‌های X25519 و P-256)
- امکان افزودن هدرهای سفارشی برای ابهام‌سازی بیشتر (در پیاده‌سازی وب‌سوکت)

مزایا:
- شباهت به ترافیک HTTPS/WS معمول و کاهش ریسک تشخیص توسط DPI
- انعطاف‌پذیری و قابلیت گسترش الگوها
- سازگاری عقب‌رو با کلاینت‌های بدون ابهام‌سازی

محدودیت‌ها:
- هیچ روش ابهام‌سازی‌ای کامل نیست؛ DPIهای پیشرفته ممکن است همچنان توانایی تشخیص داشته باشند
- سربار اندک پردازشی و نیاز به به‌روزرسانی دوره‌ای مجموعه User-Agent ها

برای درک عمیق‌تر، سند `OBFUSCATION.md` را ببینید.

---

### تولید خودکار گواهی خودامضاء (Self-signed TLS)
برای سناریوهای تست یا محیط‌هایی که گواهی معتبر در دسترس نیست، می‌توانید به‌سادگی یک گواهی خودامضاء بسازید. نمونه دستورات در همین فایل آمده است. همچنین در صورت خالی‌بودن مسیرهای TLS در سرور، مقادیر پیش‌فرض به صورت خودکار روی مسیرهای زیر تنظیم می‌شوند:
- پوشه کاری/ssl/server.crt
- پوشه کاری/ssl/server.key

پیشنهاد عملیاتی: در محیط تولید حتماً از گواهی معتبر استفاده کنید و کلید خصوصی را با دسترسی محدود نگه‌داری کنید.

---

### PPROF (پروفایلینگ)
برای عیب‌یابی و پروفایلینگ، PPROF را می‌توانید در سرور و کلاینت فعال کنید:
- سرور: وقتی `PPROF = true` باشد، سرویس روی `0.0.0.0:6060` بالا می‌آید
- کلاینت: وقتی `PPROF = true` باشد، سرویس روی `0.0.0.0:6061` بالا می‌آید
با ابزارهای استاندارد Go/pprof یا مرورگر می‌توانید به این آدرس‌ها وصل شوید. فعال‌سازی را فقط در محیط امن انجام دهید.

---

### API‌های نظارتی وب‌پنل
وقتی `web_port > 0` باشد:
- `/` صفحه داشبورد HTML
- `/stats` خروجی JSON از آمار سیستم و وضعیت تونل (CPU/RAM/Disk/Swap/Traffic/Connections/Status)
- `/data` خروجی JSON مصرف پورت‌ها (اگر `sniffer=true`)
- `/config` کانفیگ جاری بدون فیلدهای حساس (پارامتر `?type=client` برای دریافت کانفیگ کلاینت)

نمونه مصرف `/stats`:
```bash
curl http://127.0.0.1:2060/stats | jq .
```

---

### جزئیات تیونینگ خودکار (گسترده)
منطق تیونینگ در `internal/tuning/tuner.go` اجرا می‌شود و به‌صورت دوره‌ای (قابل تغییر با `--tune-interval`) شاخص‌های زیر را می‌سنجد:
- بار CPU، مصرف RAM
- تاخیر شبکه (TCP Dial) و تاریخچه آن (میانگین/واریانس)
- Packet Loss و Throughput تقریبی

پارامترهای تنظیم‌شونده:
- کلاینت: `connection_pool`
- سرور: `channel_size`, `mux_framesize`, `mux_recievebuffer`, `mux_streambuffer`, `heartbeat`, `mux_con`
- هر دو: `keepalive_period` (همگام‌سازی سمت کلاینت با وب‌پنل سرور نیز انجام می‌شود)

تنظیم پویا بازه تیونینگ:
- در نوسان بالای تاخیر، بازه کوتاه‌تر می‌شود (تیونینگ سریع‌تر)
- در پایداری بالا، بازه بلندتر می‌شود (سربار کمتر)

---

### Hot Reload (بارگذاری مجدد امن کانفیگ)
پرونده‌ی کانفیگ مشخص‌شده با `-c` مانیتور می‌شود. به محض تغییر زمان ویرایش:
1) اگر تیونینگ فعال است، Tuner فعلی متوقف می‌شود
2) کانتکست اجرای قبلی لغو و instance جدید با کانتکست تازه راه‌اندازی می‌شود
3) تیونینگ (در صورت فعال بودن) با مقادیر جدید مجدداً شروع می‌شود

اگر طی 5 ثانیه خاموشی امن کامل نشود، خاموشی اجباری اعمال می‌شود.

---

### اسکریپت نصب خودکار (`installer.sh`) – کامل
قابلیت‌ها:
- نصب Online/Offline وابستگی‌ها (apt) و مدیریت آرشیو آفلاین
- دانلود/استخراج بسته باینری و ایجاد ساختار `/root/backhaul_pro`
- ایجاد/ویرایش `config.toml` (Server/Client) با اعتبارسنجی IP/Port و بررسی اشغال‌بودن پورت‌ها
- انتخاب پروتکل: `udp`, `tcp`, `tcpmux`, `ws`, `wss`, `wsmux`, `wssmux`
- تولید توکن تصادفی یا دریافت توکن سفارشی
- تنظیم `web_port` و تعریف `ports` برای سرور (با الگوهای `443-600`, `443-600:5201`, `127.0.0.2:443=1.1.1.1:5201`, ...)
- ساخت سرویس systemd (شماره‌دار در صورت چند کانفیگ)
- راه‌اندازی و فعال‌سازی سرویس + نمایش وضعیت
- نصب ابزار مدیریت مرکزی `bh-p`

ورود به منوی مدیریت مرکزی:
```bash
sudo bh-p
```
امکانات منو:
- نمایش وضعیت سرویس، لاگ زنده، ری‌استارت
- ویرایش تعاملی کانفیگ و اعمال خودکار
- نمایش اطلاعات اتصال سمت سرور (Server Connection Info)
- حذف سرویس انتخابی (Uninstall) با پاک‌سازی هوشمند فایل‌ها و سرویس

حذف کامل سرویس‌ها (Batch):
```bash
sudo bash installer.sh uninstall
```
اسکریپت سرویس‌هایی با الگوهای `backhaul_pro*.service` و `utunnel*.service` را تشخیص و مدیریت می‌کند. در صورت نبود فایل‌ها/باینری باقی‌مانده، پوشه کاری نیز قابل حذف است. اگر هیچ سرویس مرتبط باقی نماند، دستور `bh-p` نیز حذف می‌شود.

---

### ریزقابلیت‌ها و تنظیمات کمتر مشهود
- `accept_udp` (سرور TCP): عبور UDP روی تونل TCP
- `channel_size` (سرور): ظرفیت صف پیام‌ها؛ از افت بسته جلوگیری/یا کنترل ازدحام
- `connection_pool` (کلاینت): پیش‌اتصال‌ها برای کاهش تاخیر اولیه؛ در حالت aggressive مدیریت تهاجمی‌تر است
- `nodelay`: فعال‌سازی TCP_NODELAY برای بهبود تاخیر (ممکن است پهنای باند مؤثر را کمی کاهش دهد)
- `mux_session`, `mux_version`: پارامترهای SMUX (با پیش‌فرض‌های امن و کارای تعریف‌شده)
- Sniffer: ذخیره JSON مرتب‌شده از مصرف پورت‌ها و تبدیل خودکار به مقادیر خوانا (KB/MB/GB)
- Logger رنگی: سطح‌بندی قابل تنظیم با `log_level`

---
