package web

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/musix/backhaul/internal/tuning"
)

// HealthScore نشان‌دهنده‌ی وضعیت سلامت سیستم است
type HealthScore struct {
	ResourceScore int   `json:"resource_score"` // ۱-۱۰۰: وضعیت منابع (CPU, RAM, etc)
	NetworkScore  int   `json:"network_score"`  // ۱-۱۰۰: وضعیت شبکه (Latency, Loss, etc)
	Timestamp     int64 `json:"timestamp"`      // زمان محاسبه
}

// CalculateResourceScore محاسبه نمره منابع سیستم
func CalculateResourceScore(cpuUsage, memUsage float64) int {
	// هر دو باید بین ۰ تا ۱۰۰ باشند
	if cpuUsage < 0 {
		cpuUsage = 0
	}
	if cpuUsage > 100 {
		cpuUsage = 100
	}
	if memUsage < 0 {
		memUsage = 0
	}
	if memUsage > 100 {
		memUsage = 100
	}

	// میانگین وزنی: CPU ۶۰٪، RAM ۴۰٪
	avgUsage := (cpuUsage * 0.6) + (memUsage * 0.4)

	// تبدیل استفاده به نمره (استفاده کم = نمره بالا)
	// 0% استفاده = 100 نمره
	// 50% استفاده = 50 نمره
	// 100% استفاده = 0 نمره
	score := 100 - int(avgUsage)

	if score < 1 {
		score = 1
	}
	if score > 100 {
		score = 100
	}

	return score
}

// CalculateNetworkScore محاسبه نمره شبکه
func CalculateNetworkScore(latency, packetLoss, throughput, variance float64) int {
	var score float64 = 100

	// تأثیر تأخیر (Latency)
	// 0ms = 0 کاهش، 50ms = -10، 200ms = -30، 1000ms+ = -50
	if latency > 0 {
		latencyPenalty := (latency / 20) // هر 20ms = 1 نقطه کاهش
		if latencyPenalty > 50 {
			latencyPenalty = 50
		}
		score -= latencyPenalty
	}

	// تأثیر ضرر بسته‌ها (Packet Loss)
	// 0% = 0 کاهش، 1% = -20، 5% = -50، 10%+ = -80
	if packetLoss > 0 {
		lossPenalty := packetLoss * 20
		if lossPenalty > 80 {
			lossPenalty = 80
		}
		score -= lossPenalty
	}

	// تأثیر نوسان شبکه (Variance/Stability)
	// نوسان کم = بهتر
	// variance > 50 = -15
	if variance > 50 {
		variancePenalty := (variance - 50) / 10 // هر 10 واحد = 1 نقطه
		if variancePenalty > 20 {
			variancePenalty = 20
		}
		score -= variancePenalty
	}

	// تأثیر توان عبور (Throughput)
	// throughput < 1Mbps = نشان‌دهنده‌ی مشکل
	if throughput > 0 && throughput < 1 {
		throughputPenalty := (1 - throughput) * 20
		if throughputPenalty > 20 {
			throughputPenalty = 20
		}
		score -= throughputPenalty
	}

	if score < 1 {
		score = 1
	}
	if score > 100 {
		score = 100
	}

	return int(score)
}

// CalculateHealthScore محاسبه‌ی نمره‌های کلی سلامت
func CalculateHealthScore(cpuUsage, memUsage, latency, packetLoss, throughput, variance float64) *HealthScore {
	resourceScore := CalculateResourceScore(cpuUsage, memUsage)
	networkScore := CalculateNetworkScore(
		latency,
		packetLoss,
		throughput,
		variance,
	)

	return &HealthScore{
		ResourceScore: resourceScore,
		NetworkScore:  networkScore,
		Timestamp:     getCurrentTimestamp(),
	}
}

// HandleHealthScore handler برای دریافت نمره‌های سلامت (Hybrid: سرور + کلاینت)
func HandleHealthScore(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "Method not allowed",
		})
		return
	}

	// جمع‌آوری معیارهای سیستم (مقادیر واقعی از tuning package)
	cpuUsage, err := tuning.GetCPUUsage()
	if err != nil {
		cpuUsage = 0
	}

	memUsage, err := tuning.GetMemoryUsage()
	if err != nil {
		memUsage = 0
	}

	packetLoss, throughput, err := tuning.GetPacketLossAndThroughput()
	if err != nil {
		packetLoss = 0
		throughput = 0
	}

	// اندازه‌گیری latency شبکه
	latency, err := tuning.GetNetworkLatency()
	if err != nil {
		latency = 0
	}

	// محاسبه نمره‌های سرور (یا این سیستم)
	localScore := CalculateHealthScore(
		cpuUsage,
		memUsage,
		latency,
		packetLoss,
		throughput,
		0, // variance = 0 فعلاً
	)

	// دریافت نمره‌های کلاینت (یا remote) از Tuner اگر موجود باشد
	var remoteResourceScore, remoteNetworkScore int
	if tuner := tuning.GlobalTuner; tuner != nil {
		remoteResourceScore, remoteNetworkScore = tuner.GetClientMetrics()
	}

	// محاسبه نمره‌های Hybrid (میانگین Local و Remote)
	// اگر Remote موجود نبود، فقط Local استفاده می‌شود
	hybridResourceScore := localScore.ResourceScore
	hybridNetworkScore := localScore.NetworkScore

	if remoteResourceScore > 0 || remoteNetworkScore > 0 {
		// اگر remote metrics موجود است، میانگین را محاسبه کن
		hybridResourceScore = (localScore.ResourceScore + remoteResourceScore) / 2
		hybridNetworkScore = (localScore.NetworkScore + remoteNetworkScore) / 2
	}

	// Response نهایی با نمره‌های Hybrid
	response := map[string]interface{}{
		"resource_score": hybridResourceScore,
		"network_score":  hybridNetworkScore,
		"timestamp":      getCurrentTimestamp(),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// getCurrentTimestamp بازگرداندن timestamp فعلی (Unix)
func getCurrentTimestamp() int64 {
	return time.Now().Unix()
}
