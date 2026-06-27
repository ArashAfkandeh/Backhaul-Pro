#!/bin/bash

# Config Update API Usage Examples
# یک‌ریز نمونه‌هایی برای استفاده از Config Update API

WEB_PORT=7777
BASE_URL="http://localhost:$WEB_PORT/api/config"
TOKEN="your-secret-token"  # جایگزین با token واقعی از کانفیگ

echo "========================================="
echo "Config Update API Examples"
echo "========================================="
echo ""
echo "⚠️  نکته: TOKEN را با مقدار واقعی از کانفیگ جایگزین کنید"
echo ""

# مثال 1: تغییر Log Level
echo "1. تغییر Log Level سرور به debug:"
echo "---"
curl -s -X POST "$BASE_URL" \
  -H "Content-Type: application/json" \
  -d "{
    \"type\": \"server\",
    \"token\": \"$TOKEN\",
    \"log_level\": \"debug\"
  }" | jq .
echo ""
echo ""

# مثال 2: فعال کردن Sniffer
echo "2. فعال کردن Sniffer و تنظیم Heartbeat:"
echo "---"
curl -s -X POST "$BASE_URL" \
  -H "Content-Type: application/json" \
  -d "{
    \"type\": \"server\",
    \"token\": \"$TOKEN\",
    \"sniffer\": true,
    \"heartbeat\": 30
  }" | jq .
echo ""
echo ""

# مثال 3: تغییر تنظیمات Multiplexing
echo "3. تغییر تنظیمات Multiplexing:"
echo "---"
curl -s -X PUT "$BASE_URL" \
  -H "Content-Type: application/json" \
  -d "{
    \"type\": \"server\",
    \"token\": \"$TOKEN\",
    \"mux_version\": 2,
    \"mux_framesize\": 65536,
    \"mux_session\": 2,
    \"mux_recievebuffer\": 134217728
  }" | jq .
echo ""
echo ""

# مثال 4: تغییر تنظیمات TCP
echo "4. تغییر تنظیمات TCP (Nodelay و Keepalive):"
echo "---"
curl -s -X POST "$BASE_URL" \
  -H "Content-Type: application/json" \
  -d "{
    \"type\": \"server\",
    \"token\": \"$TOKEN\",
    \"nodelay\": true,
    \"keepalive_period\": 60
  }" | jq .
echo ""
echo ""

# مثال 5: تغییر Channel Size
echo "5. تغییر Channel Size:"
echo "---"
curl -s -X POST "$BASE_URL" \
  -H "Content-Type: application/json" \
  -d "{
    \"type\": \"server\",
    \"token\": \"$TOKEN\",
    \"channel_size\": 4096
  }" | jq .
echo ""
echo ""

# مثال 6: تغییر تنظیمات کلاینت
echo "6. تغییر تنظیمات کلاینت (Log Level و Retry Interval):"
echo "---"
curl -s -X POST "$BASE_URL" \
  -H "Content-Type: application/json" \
  -d "{
    \"type\": \"client\",
    \"token\": \"$TOKEN\",
    \"log_level\": \"info\",
    \"retry_interval\": 5,
    \"dial_timeout\": 15,
    \"aggressive_pool\": true
  }" | jq .
echo ""
echo ""

# مثال 7: فعال کردن PPROF
echo "7. فعال کردن PPROF:"
echo "---"
curl -s -X POST "$BASE_URL" \
  -H "Content-Type: application/json" \
  -d "{
    \"type\": \"server\",
    \"token\": \"$TOKEN\",
    \"pprof\": true
  }" | jq .
echo ""
echo ""

# مثال 8: تغییر چندین تنظیم بیک‌وقت
echo "8. تغییر چندین تنظیم بیک‌وقت:"
echo "---"
curl -s -X POST "$BASE_URL" \
  -H "Content-Type: application/json" \
  -d "{
    \"type\": \"server\",
    \"token\": \"$TOKEN\",
    \"log_level\": \"debug\",
    \"sniffer\": true,
    \"heartbeat\": 45,
    \"nodelay\": true,
    \"keepalive_period\": 120,
    \"mux_version\": 2
  }" | jq .
echo ""
echo ""

# مثال 9: خطای Validation
echo "9. نمونه خطای Validation (Heartbeat نامعتبر):"
echo "---"
curl -s -X POST "$BASE_URL" \
  -H "Content-Type: application/json" \
  -d "{
    \"type\": \"server\",
    \"token\": \"$TOKEN\",
    \"heartbeat\": -5
  }" | jq .
echo ""
echo ""

# مثال 10: خطای Token نادرست
echo "10. خطای Token نادرست:"
echo "---"
curl -s -X POST "$BASE_URL" \
  -H "Content-Type: application/json" \
  -d '{
    "type": "server",
    "token": "wrong-token",
    "log_level": "debug"
  }' | jq .
echo ""
echo ""

# مثال 11: خطای Token مفقود
echo "11. خطای Token مفقود:"
echo "---"
curl -s -X POST "$BASE_URL" \
  -H "Content-Type: application/json" \
  -d '{
    "type": "server",
    "log_level": "debug"
  }' | jq .
echo ""
echo ""

# مثال 12: دریافت تنظیمات فعلی
echo "12. دریافت تنظیمات فعلی (از /config endpoint):"
echo "---"
curl -s "http://localhost:$WEB_PORT/config?type=server" | jq .
echo ""
echo ""

echo "========================================="
echo "نکات مهم:"
echo "========================================="
echo "1. ترتیب Endpoints:"
echo "   - GET  http://localhost:$WEB_PORT/config?type=server    - دریافت تنظیمات (بدون Token)"
echo "   - POST http://localhost:$WEB_PORT/api/config           - تغییر تنظیمات (با Token الزامی)"
echo ""
echo "2. Web Port پیش‌فرض: $WEB_PORT"
echo "3. TOKEN را با مقدار واقعی از کانفیگ جایگزین کنید"
echo "4. تغییرات بدون نیاز به ریستارت اعمال می‌شوند"
echo "5. تمام درخواست‌های ناموفق در لاگ ثبت می‌شوند"
echo "4. تمام تغییرات در لاگ ثبت می‌شوند"
echo ""
