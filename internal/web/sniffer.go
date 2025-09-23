package web

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/net"

	"github.com/musix/backhaul/internal/config"
	"github.com/sirupsen/logrus"
)

type Usage struct {
	dataStore    sync.Map
	listenAddr   string
	shutdownCtx  context.Context
	cancelFunc   context.CancelFunc
	server       *http.Server
	logger       *logrus.Logger
	sniffer      bool
	snifferLog   string
	mu           sync.Mutex
	totalTraffic uint64
	tunnelStatus *string
}

type PortUsage struct {
	Port  int
	Usage uint64
}

type SystemStats struct {
	TunnelStatus    string `json:"tunnelStatus"`
	CPUUsage        string `json:"cpuUsage"`
	RAMUsage        string `json:"ramUsage"`
	DiskUsage       string `json:"diskUsage"`
	SwapUsage       string `json:"swapUsage"`
	NetworkTraffic  string `json:"networkTraffic"`
	UploadSpeed     string `json:"uploadSpeed"`
	DownloadSpeed   string `json:"downloadSpeed"`
	BackhaulTraffic string `json:"backhaulTraffic"`
	Sniffer         string `json:"sniffer"`
	AllConnections  string `json:"allConnections"`
}

type ConfigProvider interface {
	GetServerConfig() *config.ServerConfig
	GetClientConfig() *config.ClientConfig
}

var configProvider ConfigProvider

func SetConfigProvider(provider ConfigProvider) {
	configProvider = provider
}

func NewDataStore(listenAddr string, shutdownCtx context.Context, snifferLog string, sniffer bool, tunnelStatus *string, logger *logrus.Logger) *Usage {
	ctx, cancel := context.WithCancel(shutdownCtx)
	u := &Usage{
		listenAddr:   listenAddr,
		shutdownCtx:  ctx,
		cancelFunc:   cancel,
		logger:       logger,
		sniffer:      sniffer,
		snifferLog:   snifferLog,
		tunnelStatus: tunnelStatus,
		mu:           sync.Mutex{},
		totalTraffic: 0,
	}
	return u
}

func handleConfig(w http.ResponseWriter, r *http.Request) {
	if configProvider == nil {
		logrus.Error("[WEB] Config provider not set!")
		http.Error(w, "Config provider not set", http.StatusInternalServerError)
		return
	}
	configType := r.URL.Query().Get("type")
	var cfg interface{}
	if configType == "client" {
		orig := configProvider.GetClientConfig()
		if orig == nil {
			// اگر سرور هستیم و کانفیگ کلاینت نداریم، یک آبجکت خالی برگردان و خطا لاگ نکن
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte("{}"))
			return
		}
		// Copy all fields except Token
		cfg = struct {
			RemoteAddr       string `json:"remote_addr"`
			Transport        string `json:"transport"`
			RetryInterval    int    `json:"retry_interval"`
			Nodelay          bool   `json:"nodelay"`
			Keepalive        int    `json:"keepalive_period"`
			LogLevel         string `json:"log_level"`
			PPROF            bool   `json:"pprof"`
			MuxSession       int    `json:"mux_session"`
			MuxVersion       int    `json:"mux_version"`
			MaxFrameSize     int    `json:"mux_framesize"`
			MaxReceiveBuffer int    `json:"mux_recievebuffer"`
			MaxStreamBuffer  int    `json:"mux_streambuffer"`
			Sniffer          *bool  `json:"sniffer"`
			WebPort          int    `json:"web_port"`
			SnifferLog       string `json:"sniffer_log"`
			DialTimeout      int    `json:"dial_timeout"`
			AggressivePool   bool   `json:"aggressive_pool"`
			EdgeIP           string `json:"edge_ip"`
			ConnectionPool   int    `json:"connection_pool"`
		}{
			RemoteAddr:       orig.RemoteAddr,
			Transport:        string(orig.Transport),
			RetryInterval:    orig.RetryInterval,
			Nodelay:          orig.Nodelay,
			Keepalive:        orig.Keepalive,
			LogLevel:         orig.LogLevel,
			PPROF:            orig.PPROF,
			MuxSession:       orig.MuxSession,
			MuxVersion:       orig.MuxVersion,
			MaxFrameSize:     orig.MaxFrameSize,
			MaxReceiveBuffer: orig.MaxReceiveBuffer,
			MaxStreamBuffer:  orig.MaxStreamBuffer,
			Sniffer:          orig.Sniffer,
			WebPort:          orig.WebPort,
			SnifferLog:       orig.SnifferLog,
			DialTimeout:      orig.DialTimeout,
			AggressivePool:   orig.AggressivePool,
			EdgeIP:           orig.EdgeIP,
			ConnectionPool:   orig.ConnectionPool,
		}
	} else {
		orig := configProvider.GetServerConfig()
		if orig == nil {
			// اگر هیچ کانفیگی نبود (که نباید باشد)، فقط همینجا خطا لاگ کن
			logrus.Error("[WEB] handleConfig: server config is nil!")
			http.Error(w, "Config is nil", http.StatusNotFound)
			return
		}
		cfg = struct {
			BindAddr         string   `json:"bind_addr"`
			Transport        string   `json:"transport"`
			Nodelay          bool     `json:"nodelay"`
			Keepalive        int      `json:"keepalive_period"`
			LogLevel         string   `json:"log_level"`
			Ports            []string `json:"ports"`
			PPROF            bool     `json:"pprof"`
			MuxSession       int      `json:"mux_session"`
			MuxVersion       int      `json:"mux_version"`
			MaxFrameSize     int      `json:"mux_framesize"`
			MaxReceiveBuffer int      `json:"mux_recievebuffer"`
			MaxStreamBuffer  int      `json:"mux_streambuffer"`
			Sniffer          *bool    `json:"sniffer"`
			WebPort          int      `json:"web_port"`
			SnifferLog       string   `json:"sniffer_log"`
			TLSCertFile      string   `json:"tls_cert"`
			TLSKeyFile       string   `json:"tls_key"`
			Heartbeat        int      `json:"heartbeat"`
			MuxCon           int      `json:"mux_con"`
			AcceptUDP        bool     `json:"accept_udp"`
			ChannelSize      int      `json:"channel_size"`
		}{
			BindAddr:         orig.BindAddr,
			Transport:        string(orig.Transport),
			Nodelay:          orig.Nodelay,
			Keepalive:        orig.Keepalive,
			LogLevel:         orig.LogLevel,
			Ports:            orig.Ports,
			PPROF:            orig.PPROF,
			MuxSession:       orig.MuxSession,
			MuxVersion:       orig.MuxVersion,
			MaxFrameSize:     orig.MaxFrameSize,
			MaxReceiveBuffer: orig.MaxReceiveBuffer,
			MaxStreamBuffer:  orig.MaxStreamBuffer,
			Sniffer:          orig.Sniffer,
			WebPort:          orig.WebPort,
			SnifferLog:       orig.SnifferLog,
			TLSCertFile:      orig.TLSCertFile,
			TLSKeyFile:       orig.TLSKeyFile,
			Heartbeat:        orig.Heartbeat,
			MuxCon:           orig.MuxCon,
			AcceptUDP:        orig.AcceptUDP,
			ChannelSize:      orig.ChannelSize,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(cfg); err != nil {
		logrus.WithError(err).Error("[WEB] handleConfig: Failed to encode config")
		http.Error(w, "Failed to encode config", http.StatusInternalServerError)
	}
}

func (m *Usage) Monitor() {
	mux := http.NewServeMux()
	mux.HandleFunc("/", m.handleIndex) // handle index
	mux.HandleFunc("/stats", m.statsHandler)
	if m.sniffer {
		mux.HandleFunc("/data", m.handleData) // New route for JSON data
	}
	mux.HandleFunc("/config", handleConfig) // New endpoint for config
	m.server = &http.Server{
		Addr:    m.listenAddr,
		Handler: mux,
	}

	go func() {
		<-m.shutdownCtx.Done()

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		// Attempt to gracefully shut down the server
		if err := m.server.Shutdown(shutdownCtx); err != nil {
			m.logger.Errorf("sniffer server shutdown error: %v", err)
		}
	}()

	// start save data
	if m.sniffer {
		go func() {
			ticker := time.NewTicker(15 * time.Second) // every 5 seconds
			defer ticker.Stop()

			for {
				select {
				case <-ticker.C:
					go m.saveUsageData()
				case <-m.shutdownCtx.Done():
					return
				}
			}
		}()
	}
	// Start the server
	m.logger.Info("sniffer service listening on port: ", m.listenAddr)
	if err := m.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		m.logger.Errorf("sniffer server error: %v", err)
	}
}

//go:embed index.html
var indexHTML embed.FS

func (m *Usage) handleIndex(w http.ResponseWriter, r *http.Request) {
	usageData := m.getUsageFromFile()
	readableData := m.usageDataWithReadableUsage(usageData)

	tmpl, err := template.ParseFS(indexHTML, "index.html")
	if err != nil {
		m.logger.Errorf("error parsing template: %v", err)
		return
	}

	err = tmpl.Execute(w, readableData)
	if err != nil {
		m.logger.Errorf("error executing template: %v", err)
	}
}

func (m *Usage) handleData(w http.ResponseWriter, r *http.Request) {
	usageData := m.getUsageFromFile()
	readableData := m.usageDataWithReadableUsage(usageData)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(readableData); err != nil {
		m.logger.Errorf("error encoding JSON response: %v", err)
	}
}

func (m *Usage) statsHandler(w http.ResponseWriter, r *http.Request) {
	stats, err := m.getSystemStats()
	if err != nil {
		m.logger.Error("Error fetching system stats:", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(stats); err != nil {
		m.logger.Error("Error encoding JSON:", err)
	}
}

func (m *Usage) AddOrUpdatePort(port int, usage uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Retrieve current usage data for the port
	value, ok := m.dataStore.Load(port)
	if ok {
		// Port exists, update usage
		portUsage := value.(PortUsage)
		portUsage.Usage += usage
		m.dataStore.Store(port, portUsage)
	} else {
		// Port does not exist, create new entry
		m.dataStore.Store(port, PortUsage{Port: port, Usage: usage})
	}
}

func (m *Usage) saveUsageData() {
	// Step 1: Load existing usage data from the JSON file
	var existingUsageData []PortUsage
	file, err := os.Open(m.snifferLog)
	if err == nil {
		// If the file exists, decode the JSON data into existingUsageData
		defer file.Close()

		// Check if file is empty
		fileInfo, err := file.Stat()
		if err != nil {
			m.logger.Errorf("error getting file info: %v", err)
			return
		}

		if fileInfo.Size() == 0 {
			// File is empty, start with empty slice
			existingUsageData = []PortUsage{}
		} else {
			// Try to decode JSON data
			err = json.NewDecoder(file).Decode(&existingUsageData)
			if err != nil {
				m.logger.Errorf("error decoding JSON data: %v", err)
				// If decoding fails, clean up the corrupted file
				m.cleanupCorruptedFile()
				existingUsageData = []PortUsage{}
				m.logger.Info("Starting with empty usage data due to JSON decode error")
			}
		}
	} else if !os.IsNotExist(err) {
		// Log any error except file not existing
		m.logger.Errorf("error opening JSON file: %v", err)
		return
	}

	// Step 2: Get current usage data from sync.Map
	currentUsageData := m.collectUsageDataFromSyncMap()

	// Step 3: Merge the existing and current usage data into a map to avoid duplicates
	usageMap := make(map[int]PortUsage)

	// Add existing usage data to the map
	for _, usage := range existingUsageData {
		usageMap[usage.Port] = usage
	}

	// Append or update current usage data in the map
	for _, usage := range currentUsageData {
		if existing, exists := usageMap[usage.Port]; exists {
			// Update existing port usage
			existing.Usage += usage.Usage
			usageMap[usage.Port] = existing
		} else {
			// Add new port usage
			usageMap[usage.Port] = usage
		}
	}

	m.totalTraffic = 0

	// Step 4: Convert the map back to a slice
	var mergedUsageData []PortUsage
	for _, usage := range usageMap {
		mergedUsageData = append(mergedUsageData, usage)
		m.totalTraffic += usage.Usage
	}

	// Step 5: Convert merged data to JSON
	data, err := json.MarshalIndent(mergedUsageData, "", "  ")
	if err != nil {
		m.logger.Errorf("error marshalling usage data: %v", err)
		return
	}

	// Step 6: Write JSON data to file
	err = os.WriteFile(m.snifferLog, data, 0644)
	if err != nil {
		m.logger.Errorf("error writing usage data to file: %v", err)
	}
}

func (m *Usage) getUsageFromFile() []PortUsage {
	// Check if the file exists
	if _, err := os.Stat(m.snifferLog); os.IsNotExist(err) {
		// If the file does not exist, create it and write empty array
		file, err := os.OpenFile(m.snifferLog, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
		if err != nil {
			m.logger.Errorf("error creating file: %v", err)
			return nil
		}
		defer file.Close()

		// Write empty array to the new file
		if _, err := file.Write([]byte("[]")); err != nil {
			m.logger.Errorf("error writing empty array to the file: %v", err)
			return nil
		}

		return []PortUsage{}
	}

	var usageData []PortUsage

	// Open the JSON file
	file, err := os.Open(m.snifferLog)
	if err != nil {
		m.logger.Errorf("error opening JSON file: %v", err)
		return nil
	}
	defer file.Close()

	// Check if file is empty
	fileInfo, err := file.Stat()
	if err != nil {
		m.logger.Errorf("error getting file info: %v", err)
		return nil
	}

	if fileInfo.Size() == 0 {
		// File is empty, return empty slice
		return []PortUsage{}
	}

	// Decode the JSON file into the usageData slice
	err = json.NewDecoder(file).Decode(&usageData)
	if err != nil {
		m.logger.Errorf("error decoding JSON data: %v", err)
		// Return empty slice instead of nil to prevent further errors
		return []PortUsage{}
	}

	// Sort usageData by Port in ascending order
	sort.Slice(usageData, func(i, j int) bool {
		return usageData[i].Port < usageData[j].Port
	})

	return usageData
}

// converts the byte usage to a human-readable format
func (m *Usage) usageDataWithReadableUsage(usageData []PortUsage) []struct {
	Port          int
	ReadableUsage string
} {
	var result []struct {
		Port          int
		ReadableUsage string
	}

	for _, portUsage := range usageData {
		result = append(result, struct {
			Port          int
			ReadableUsage string
		}{
			Port:          portUsage.Port,
			ReadableUsage: m.convertBytesToReadable(portUsage.Usage),
		})
	}

	return result
}

// collectUsageDataFromSyncMap gathers data from sync.Map
func (m *Usage) collectUsageDataFromSyncMap() []PortUsage {
	m.mu.Lock()
	defer m.mu.Unlock()

	var usageData []PortUsage
	m.dataStore.Range(func(key, value interface{}) bool {
		if portUsage, ok := value.(PortUsage); ok {
			usageData = append(usageData, portUsage)
			m.dataStore.Delete(key)
		}
		return true
	})
	return usageData
}

// ConvertBytesToReadable converts bytes into a human-readable format (KB, MB, GB)
func (m *Usage) convertBytesToReadable(bytes uint64) string {
	const (
		KB = 1 << (10 * 1) // 1024 bytes
		MB = 1 << (10 * 2) // 1024 KB
		GB = 1 << (10 * 3) // 1024 MB
		TB = 1 << (10 * 4) // 1024 TB
	)

	switch {
	case bytes >= TB:
		return fmt.Sprintf("%.2f TB", float64(bytes)/float64(TB))
	case bytes >= GB:
		return fmt.Sprintf("%.2f GB", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.2f MB", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.2f KB", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%d B", bytes) // Bytes
	}
}

func (m *Usage) getSystemStats() (*SystemStats, error) {

	// Get initial network stats
	initialStats, err := m.getNetworkStats()
	if err != nil {
		return nil, err
	}

	// Wait for 1 second
	time.Sleep(1 * time.Second)

	// Get updated network stats
	finalStats, err := m.getNetworkStats()
	if err != nil {
		return nil, err
	}

	// Get CPU usage
	cpuPercent, err := cpu.Percent(0, false)
	if err != nil {
		return nil, err
	}

	// Get RAM usage
	memStats, err := mem.VirtualMemory()
	if err != nil {
		return nil, err
	}

	// Get Disk usage
	diskStats, err := disk.Usage("/")
	if err != nil {
		return nil, err
	}

	// Get Swap usage
	swapStats, err := mem.SwapMemory()
	if err != nil {
		return nil, err
	}

	// Get Network traffic
	netStats, err := net.IOCounters(false)
	if err != nil {
		return nil, err
	}

	// Get all active network connections (TCP, UDP, etc.)
	connections, err := net.Connections("all")
	if err != nil {
		return nil, err
	}

	// Calculate upload and download speeds
	uploadSpeed := float64(finalStats.BytesSent - initialStats.BytesSent)
	downloadSpeed := float64(finalStats.BytesRecv - initialStats.BytesRecv)

	stats := &SystemStats{
		TunnelStatus:    *m.tunnelStatus,
		CPUUsage:        m.formatFloat(cpuPercent[0]),
		RAMUsage:        m.convertBytesToReadable(memStats.Used),
		DiskUsage:       m.convertBytesToReadable(diskStats.Used),
		SwapUsage:       m.convertBytesToReadable(swapStats.Used),
		NetworkTraffic:  m.convertBytesToReadable(netStats[0].BytesSent + netStats[0].BytesRecv),
		DownloadSpeed:   m.formatSpeed(downloadSpeed),
		UploadSpeed:     m.formatSpeed(uploadSpeed),
		BackhaulTraffic: m.convertBytesToReadable(m.totalTraffic),
		Sniffer:         map[bool]string{true: "Running", false: "Not running"}[m.sniffer],
		AllConnections:  fmt.Sprintf("%d", len(connections)),
	}

	return stats, nil
}

func (m *Usage) formatSpeed(bytesPerSec float64) string {
	if bytesPerSec >= 1e9 {
		return fmt.Sprintf("%.2f GB/s", bytesPerSec/1e9)
	} else if bytesPerSec >= 1e6 {
		return fmt.Sprintf("%.2f MB/s", bytesPerSec/1e6)
	} else if bytesPerSec >= 1e3 {
		return fmt.Sprintf("%.2f KB/s", bytesPerSec/1e3)
	}
	return fmt.Sprintf("%.2f B/s", bytesPerSec)
}

func (m *Usage) formatFloat(value float64) string {
	return fmt.Sprintf("%.2f%%", value)
}

func (m *Usage) getNetworkStats() (*net.IOCountersStat, error) {
	ioCounters, err := net.IOCounters(false)
	if err != nil {
		return nil, err
	}
	if len(ioCounters) == 0 {
		return nil, fmt.Errorf("no network IO counters found")
	}
	return &ioCounters[0], nil
}

// cleanupCorruptedFile cleans up a corrupted JSON file by recreating it
func (m *Usage) cleanupCorruptedFile() {
	m.logger.Warn("Attempting to clean up corrupted JSON file")

	// Remove the corrupted file
	if err := os.Remove(m.snifferLog); err != nil {
		m.logger.Errorf("error removing corrupted file: %v", err)
		return
	}

	// Create a new file with empty array
	file, err := os.OpenFile(m.snifferLog, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		m.logger.Errorf("error creating new file: %v", err)
		return
	}
	defer file.Close()

	// Write empty array to the new file
	if _, err := file.Write([]byte("[]")); err != nil {
		m.logger.Errorf("error writing empty array to new file: %v", err)
		return
	}

	m.logger.Info("Successfully cleaned up corrupted JSON file")
}
