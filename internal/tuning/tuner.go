package tuning

import (
	"context"
	"net"
	"sync"
	"time"

	"github.com/musix/backhaul/internal/config"
	"github.com/sirupsen/logrus"
)

// Tuner dynamically adjusts application parameters based on system metrics.
type Tuner struct {
	config *config.Config
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	logger *logrus.Logger
	ticker *time.Ticker

	// Adaptive interval management
	baseInterval    time.Duration
	currentInterval time.Duration
	lastLatency     float64
	latencyHistory  []float64
	historySize     int
	mu              sync.RWMutex
}

// NewTuner creates a new Tuner instance.
func NewTuner(cfg *config.Config, logger *logrus.Logger) *Tuner {
	ctx, cancel := context.WithCancel(context.Background())
	return &Tuner{
		config:          cfg,
		ctx:             ctx,
		cancel:          cancel,
		logger:          logger,
		baseInterval:    10 * time.Minute, // Default base interval
		currentInterval: 10 * time.Minute,
		latencyHistory:  make([]float64, 0, 10),
		historySize:     10,
	}
}

// Start begins the dynamic tuning process.
func (t *Tuner) Start(interval time.Duration) {
	t.baseInterval = interval
	t.currentInterval = interval
	t.ticker = time.NewTicker(interval)
	t.wg.Add(1)
	go func() {
		defer t.wg.Done()
		for {
			select {
			case <-t.ticker.C:
				t.adjustParameters()
				t.adjustInterval()
			case <-t.ctx.Done():
				t.ticker.Stop()
				return
			}
		}
	}()
	t.logger.WithField("interval", interval).Info("Dynamic tuner started.")
}

// Stop halts the dynamic tuning process.
func (t *Tuner) Stop() {
	t.cancel()
	t.wg.Wait()
	t.logger.Info("Dynamic tuner stopped.")
}

// adjustParameters collects metrics and adjusts config values.
func (t *Tuner) adjustParameters() {
	t.logger.Info("[TUNER] adjustParameters called")
	t.logger.Debug("Running automatic parameter adjustment...")

	cpuUsage, err := GetCPUUsage()
	if err != nil {
		t.logger.WithError(err).Error("Failed to get CPU usage")
	} else {
		t.logger.WithField("usage", cpuUsage).Debug("Current CPU usage")
	}

	memUsage, err := GetMemoryUsage()
	if err != nil {
		t.logger.WithError(err).Error("Failed to get memory usage")
	} else {
		t.logger.WithField("usage", memUsage).Debug("Current memory usage")
	}

	// Adjust client connection pool (for both client and server modes)
	if t.config.Client != nil {
		if cpuUsage > 85.0 || memUsage > 85.0 {
			if t.config.Client.ConnectionPool > 1 {
				t.config.Client.ConnectionPool--
				t.logger.WithField("pool_size", t.config.Client.ConnectionPool).Info("Decreased client connection pool")
			}
		} else if cpuUsage < 50.0 && memUsage < 50.0 {
			if t.config.Client.ConnectionPool < 64 { // Set a reasonable upper limit
				t.config.Client.ConnectionPool++
				t.logger.WithField("pool_size", t.config.Client.ConnectionPool).Info("Increased client connection pool")
			}
		}
	}

	// Adjust server channel size (only for server mode)
	if t.config.Server != nil {
		if memUsage > 80.0 {
			if t.config.Server.ChannelSize > 256 { // Set a reasonable lower limit
				t.config.Server.ChannelSize /= 2
				t.logger.WithField("channel_size", t.config.Server.ChannelSize).Info("Decreased server channel size")
			}
		} else if memUsage < 40.0 {
			if t.config.Server.ChannelSize < 8192 { // Set a reasonable upper limit
				t.config.Server.ChannelSize *= 2
				t.logger.WithField("channel_size", t.config.Server.ChannelSize).Info("Increased server channel size")
			}
		}
	}

	latency, err := getNetworkLatency(t.config)
	// --- Adaptive decision based on all metrics ---
	packetLoss, throughput, plErr := GetPacketLossAndThroughput()
	if plErr != nil {
		packetLoss = 0
		throughput = 0
	}

	// Adaptive adjustment factor
	adjustDown := latency > 200 || cpuUsage > 80.0 || memUsage > 80.0 || packetLoss > 2.0
	adjustUp := latency < 70 && cpuUsage < 60.0 && memUsage < 60.0 && packetLoss < 1.0 && throughput > 0

	// --- Adaptive keepalive ---
	if err == nil {
		minKeepalive := 10
		maxKeepalive := 180
		step := 5
		base := 75
		var targetKeepalive int
		if adjustDown {
			targetKeepalive = minKeepalive
		} else if adjustUp {
			targetKeepalive = maxKeepalive
		} else {
			targetKeepalive = base
		}
		if targetKeepalive < minKeepalive {
			targetKeepalive = minKeepalive
		}
		if targetKeepalive > maxKeepalive {
			targetKeepalive = maxKeepalive
		}
		serverChanged := false
		clientChanged := false
		if t.config.Server != nil {
			if absInt(t.config.Server.Keepalive-targetKeepalive) >= step {
				t.config.Server.Keepalive = targetKeepalive
				serverChanged = true
			}
		}
		if t.config.Client != nil {
			if absInt(t.config.Client.Keepalive-targetKeepalive) >= step {
				t.config.Client.Keepalive = targetKeepalive
				clientChanged = true
			}
		}
		if t.config.Server != nil && t.config.Client != nil {
			if serverChanged && !clientChanged {
				t.config.Client.Keepalive = targetKeepalive
			} else if clientChanged && !serverChanged {
				t.config.Server.Keepalive = targetKeepalive
			}
		}
		// Check synchronization status
		t.checkKeepaliveSync()
	}

	// --- Adaptive MUX parameters (server only) ---
	if t.config.Server != nil && err == nil {
		// mux_framesize
		minFrame := 16 * 1024
		maxFrame := 128 * 1024
		frameStep := 8 * 1024
		baseFrame := 32 * 1024
		var targetFrame int
		if adjustDown {
			targetFrame = minFrame
		} else if adjustUp {
			targetFrame = maxFrame
		} else {
			targetFrame = baseFrame
		}
		if targetFrame < minFrame {
			targetFrame = minFrame
		}
		if targetFrame > maxFrame {
			targetFrame = maxFrame
		}
		if absInt(t.config.Server.MaxFrameSize-targetFrame) >= frameStep {
			t.config.Server.MaxFrameSize = targetFrame
		}
		// mux_receivebuffer
		minRecv := 1 * 1024 * 1024
		maxRecv := 16 * 1024 * 1024
		recvStep := 1 * 1024 * 1024
		baseRecv := 4 * 1024 * 1024
		var targetRecv int
		if adjustDown {
			targetRecv = minRecv
		} else if adjustUp {
			targetRecv = maxRecv
		} else {
			targetRecv = baseRecv
		}
		if targetRecv < minRecv {
			targetRecv = minRecv
		}
		if targetRecv > maxRecv {
			targetRecv = maxRecv
		}
		if absInt(t.config.Server.MaxReceiveBuffer-targetRecv) >= recvStep {
			t.config.Server.MaxReceiveBuffer = targetRecv
		}
		// mux_streambuffer
		minStream := 64 * 1024
		maxStream := 1024 * 1024
		streamStep := 64 * 1024
		baseStream := 256 * 1024
		var targetStream int
		if adjustDown {
			targetStream = minStream
		} else if adjustUp {
			targetStream = maxStream
		} else {
			targetStream = baseStream
		}
		if targetStream < minStream {
			targetStream = minStream
		}
		if targetStream > maxStream {
			targetStream = maxStream
		}
		if absInt(t.config.Server.MaxStreamBuffer-targetStream) >= streamStep {
			t.config.Server.MaxStreamBuffer = targetStream
		}
	}

	// --- Adaptive heartbeat (server only) ---
	if t.config.Server != nil {
		heartbeatMin := 10
		heartbeatMax := 120
		heartbeatStep := 15
		heartbeatBase := 40
		var targetHeartbeat int
		if adjustDown {
			targetHeartbeat = heartbeatMax
		} else if adjustUp {
			targetHeartbeat = heartbeatMin
		} else {
			targetHeartbeat = heartbeatBase
		}
		if targetHeartbeat < heartbeatMin {
			targetHeartbeat = heartbeatMin
		}
		if targetHeartbeat > heartbeatMax {
			targetHeartbeat = heartbeatMax
		}
		if absInt(t.config.Server.Heartbeat-targetHeartbeat) >= heartbeatStep {
			t.logger.WithFields(logrus.Fields{
				"old":         t.config.Server.Heartbeat,
				"new":         targetHeartbeat,
				"latency":     latency,
				"cpu":         cpuUsage,
				"mem":         memUsage,
				"packet_loss": packetLoss,
				"throughput":  throughput,
			}).Info("[TUNER] Adaptive heartbeat updated")
			t.config.Server.Heartbeat = targetHeartbeat
		}

		// --- Adaptive mux_con (server only) ---
		muxConMin := 2
		muxConMax := 32
		muxConStep := 2
		muxConBase := 8
		var targetMuxCon int
		if adjustDown {
			targetMuxCon = muxConMin
		} else if adjustUp {
			targetMuxCon = muxConMax
		} else {
			targetMuxCon = muxConBase
		}
		if targetMuxCon < muxConMin {
			targetMuxCon = muxConMin
		}
		if targetMuxCon > muxConMax {
			targetMuxCon = muxConMax
		}
		if absInt(t.config.Server.MuxCon-targetMuxCon) >= muxConStep {
			t.logger.WithFields(logrus.Fields{
				"old":         t.config.Server.MuxCon,
				"new":         targetMuxCon,
				"latency":     latency,
				"cpu":         cpuUsage,
				"mem":         memUsage,
				"packet_loss": packetLoss,
				"throughput":  throughput,
			}).Info("[TUNER] Adaptive mux_con updated")
			t.config.Server.MuxCon = targetMuxCon
		}
	}
}

// adjustInterval adaptively adjusts the tuning interval based on network stability
func (t *Tuner) adjustInterval() {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Get current latency
	latency, err := getNetworkLatency(t.config)
	if err != nil {
		return // Keep current interval if latency measurement fails
	}

	// Add to history
	t.latencyHistory = append(t.latencyHistory, latency)
	if len(t.latencyHistory) > t.historySize {
		t.latencyHistory = t.latencyHistory[1:] // Remove oldest
	}

	// Calculate latency variance (network stability indicator)
	if len(t.latencyHistory) >= 3 {
		variance := calculateVariance(t.latencyHistory)
		avgLatency := calculateAverage(t.latencyHistory)

		// Adjust interval based on network stability
		var newInterval time.Duration

		// High variance = unstable network -> shorter interval
		if variance > avgLatency*0.3 { // 30% variance threshold
			newInterval = t.baseInterval / 2 // More frequent tuning
			if newInterval < 5*time.Minute {
				newInterval = 5 * time.Minute // Minimum interval
			}
		} else if variance < avgLatency*0.1 { // 10% variance threshold
			newInterval = time.Duration(float64(t.baseInterval) * 1.5) // Less frequent tuning
			if newInterval > 15*time.Minute {
				newInterval = 15 * time.Minute // Maximum interval
			}
		} else {
			newInterval = t.baseInterval // Keep base interval
		}

		// Only change interval if difference is significant
		if absDuration(t.currentInterval-newInterval) > 5*time.Second {
			t.currentInterval = newInterval
			t.ticker.Reset(newInterval)

			t.logger.WithFields(logrus.Fields{
				"old_interval": t.currentInterval,
				"new_interval": newInterval,
				"latency_avg":  avgLatency,
				"variance":     variance,
			}).Info("[TUNER] Adaptive interval adjusted")
		}
	}

	t.lastLatency = latency
}

// calculateVariance calculates the variance of latency values
func calculateVariance(values []float64) float64 {
	if len(values) < 2 {
		return 0
	}

	avg := calculateAverage(values)
	var sum float64
	for _, v := range values {
		sum += (v - avg) * (v - avg)
	}
	return sum / float64(len(values))
}

// calculateAverage calculates the average of latency values
func calculateAverage(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	var sum float64
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

// absDuration returns the absolute difference between two durations
func absDuration(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

// getNetworkLatency اندازه‌گیری latency با TCP connection
func getNetworkLatency(cfg *config.Config) (float64, error) {
	var targetHost string
	var targetPort string

	// اول سعی می‌کنیم به سرور تانل شده متصل شویم
	if cfg.Client != nil && cfg.Client.RemoteAddr != "" {
		// استخراج host و port از remote_addr
		targetHost, targetPort = extractHostAndPort(cfg.Client.RemoteAddr)
	} else if cfg.Server != nil && cfg.Server.BindAddr != "" {
		// برای server، به localhost متصل شویم
		targetHost = "127.0.0.1"
		targetPort = "8443"
	} else {
		// fallback به 8.8.8.8:53
		targetHost = "8.8.8.8"
		targetPort = "53"
	}

	// اندازه‌گیری latency با TCP connection
	start := time.Now()
	conn, err := net.DialTimeout("tcp", targetHost+":"+targetPort, 2*time.Second)
	if err != nil {
		// اگر اتصال به کلاینت شکست خورد، از 8.8.8.8 استفاده کنیم
		if targetHost != "8.8.8.8" {
			targetHost = "8.8.8.8"
			targetPort = "53"
			conn, err = net.DialTimeout("tcp", targetHost+":"+targetPort, 2*time.Second)
			if err != nil {
				return 0, err
			}
		} else {
			return 0, err
		}
	}
	defer conn.Close()

	latency := time.Since(start)
	return float64(latency.Milliseconds()), nil
}

// extractHostAndPort استخراج host و port از آدرس کامل
func extractHostAndPort(addr string) (string, string) {
	for i, char := range addr {
		if char == ':' {
			return addr[:i], addr[i+1:]
		}
	}
	// اگر port مشخص نشده، از port پیش‌فرض استفاده کنیم
	return addr, "8443"
}

func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// checkKeepaliveSync checks if server and client keepalive are synchronized
func (t *Tuner) checkKeepaliveSync() {
	if t.config.Server != nil && t.config.Client != nil {
		serverKeepalive := t.config.Server.Keepalive
		clientKeepalive := t.config.Client.Keepalive

		if serverKeepalive == clientKeepalive {
			t.logger.WithFields(logrus.Fields{
				"server_keepalive": serverKeepalive,
				"client_keepalive": clientKeepalive,
				"status":           "synchronized",
			}).Info("[TUNER] Keepalive synchronization check")
		} else {
			t.logger.WithFields(logrus.Fields{
				"server_keepalive": serverKeepalive,
				"client_keepalive": clientKeepalive,
				"difference":       absInt(serverKeepalive - clientKeepalive),
				"status":           "desynchronized",
			}).Warn("[TUNER] Keepalive desynchronization detected")
		}
	}
}
