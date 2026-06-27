package web

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// Global tracker for independent API server
var (
	independentAPIOnce   sync.Once
	independentAPIStatus = false
	globalCtx            context.Context
	globalCancel         context.CancelFunc
)

// convertToAbsolutePath تبدیل path نسبی به absolute کنار executable
func convertToAbsolutePath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	// اگر path نسبی بود، کنار executable بساز
	execPath, err := os.Executable()
	if err != nil {
		return path // اگر نتوانستیم executable path رو بگیریم، از path اصلی استفاده کن
	}
	return filepath.Join(filepath.Dir(execPath), path)
}

// getCertsDir دریافت مسیر folder certs کنار executable
func getCertsDir() string {
	// دریافت مسیر executable
	execPath, err := os.Executable()
	if err != nil {
		// اگر نتوانستیم مسیر را دریافت کنیم، از working directory استفاده کن
		return "certs"
	}
	// دریافت folder حاوی executable
	execDir := filepath.Dir(execPath)
	// برگشت مسیر certs folder
	return filepath.Join(execDir, "certs")
}

// IndependentAPIRunning returns whether the independent API server has been started.
func IndependentAPIRunning() bool {
	return independentAPIStatus
}

// StartIndependentAPI starts the web API server independently with HTTPS
// This allows the API to keep running even if the main tunnel server crashes
// configPath is used to provide config access even if main service crashes
// ctx is used for graceful shutdown and restart
func StartIndependentAPI(port int, logger *logrus.Logger, configPath string, ctx context.Context, cancel context.CancelFunc) {
	if port <= 0 {
		logger.Warn("[WEB] API port not configured, skipping independent API server")
		return
	}

	// Store global context for handlers
	globalCtx = ctx
	globalCancel = cancel

	// Only start once - idempotent
	independentAPIOnce.Do(func() {
		// Set up direct file config provider for resilience
		if configPath != "" {
			provider := NewDirectFileConfigProvider(configPath)
			SetConfigProvider(provider)
		}

		go func() {
			defer func() {
				if r := recover(); r != nil {
					logger.WithFields(logrus.Fields{
						"panic": r,
					}).Error("[WEB] Independent API server panic recovered")
				}
			}()

			// استفاده از گواهی‌های wssmux برای API
			// مسیر folder certs کنار executable
			certsDir := getCertsDir()
			certPath := filepath.Join(certsDir, "fullchain.crt")
			keyPath := filepath.Join(certsDir, "privkey.key")

			if err := ensureSelfSignedCert(certPath, keyPath, "localhost"); err != nil {
				logger.WithError(err).Error("[WEB] Failed to generate/load SSL certificate, skipping API server")
				return
			}

			// Create a new mux for the API server
			mux := http.NewServeMux()

			// Register config API endpoint
			mux.HandleFunc("/api/config", HandleConfigUpdate)

			// Register config rollback endpoint
			mux.HandleFunc("/api/rollback", HandleConfigRollback)

			// Register health score endpoint
			mux.HandleFunc("/api/health-score", HandleHealthScore)

			// Register status endpoint
			mux.HandleFunc("/api/status", HandleStatus)

			// Register restart endpoint
			mux.HandleFunc("/api/restart", HandleRestart)

			// Register health check endpoint
			mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"status":"ok"}`))
			})

			addr := fmt.Sprintf("0.0.0.0:%d", port)

			server := &http.Server{
				Addr:              addr,
				Handler:           mux,
				ReadTimeout:       15 * time.Second,
				WriteTimeout:      15 * time.Second,
				IdleTimeout:       60 * time.Second,
				MaxHeaderBytes:    1 << 20, // 1MB
				ReadHeaderTimeout: 5 * time.Second,
			}

			logger.WithFields(logrus.Fields{
				"address": addr,
				"scheme":  "HTTPS",
				"cert":    certPath,
			}).Info("[WEB] Independent API server started with SSL/TLS (resilient mode)")

			independentAPIStatus = true

			// استفاده از ListenAndServeTLS برای HTTPS
			if err := server.ListenAndServeTLS(certPath, keyPath); err != nil && err != http.ErrServerClosed {
				logger.WithFields(logrus.Fields{
					"error": err.Error(),
				}).Error("[WEB] Independent API server error")
			}
		}()
	})
}

// ensureSelfSignedCert تابع کمکی برای تولید یا بارگذاری گواهی خودامضاء
func ensureSelfSignedCert(certFile, keyFile, host string) error {
	// تبدیل path‌های نسبی به absolute
	certFile = convertToAbsolutePath(certFile)
	keyFile = convertToAbsolutePath(keyFile)

	// اگر فایل‌ها موجود هستند، از آن‌ها استفاده کنیم
	if _, err := os.Stat(certFile); err == nil {
		if _, err := os.Stat(keyFile); err == nil {
			return nil
		}
	}

	// ایجاد دایرکتوری
	if err := os.MkdirAll(filepath.Dir(certFile), 0700); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(keyFile), 0700); err != nil {
		return err
	}

	// تولید کلید RSA
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}

	// شماره سریال و تاریخ‌ها
	notBefore := time.Now()
	notAfter := notBefore.Add(365 * 24 * time.Hour)

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return err
	}

	// قالب گواهی
	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Backhaul-Pro"},
		},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	// افزودن host به گواهی
	if ip := net.ParseIP(host); ip != nil {
		template.IPAddresses = append(template.IPAddresses, ip)
	} else if host != "" {
		template.DNSNames = append(template.DNSNames, host)
	}

	// امضای گواهی
	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return err
	}

	// ذخیره گواهی
	certOut, err := os.Create(certFile)
	if err != nil {
		return err
	}
	defer certOut.Close()
	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes}); err != nil {
		return err
	}

	// ذخیره کلید خصوصی
	keyOut, err := os.Create(keyFile)
	if err != nil {
		return err
	}
	defer keyOut.Close()
	if err := pem.Encode(keyOut, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)}); err != nil {
		return err
	}

	return nil
}

// HandleStatus handler برای دریافت وضعیت برنامه
func HandleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		w.Write([]byte(`{"success":false,"message":"Only GET method is allowed"}`))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"success":true,"status":"running","message":"Backhaul-Pro application is running"}`))
}

// HandleRestart handler برای راه‌اندازی مجدد برنامه
func HandleRestart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		w.Write([]byte(`{"success":false,"message":"Only POST method is allowed"}`))
		return
	}

	// Parse JSON body to get token
	var req struct {
		Token string `json:"token"`
		Type  string `json:"type"` // "server" or "client"
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"success":false,"message":"Invalid JSON request"}`))
		return
	}

	// Verify token
	if req.Token == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"success":false,"message":"Authentication token is required"}`))
		return
	}

	// Check token against config (same logic as ConfigUpdate)
	if err := verifyRestartToken(req.Token, req.Type); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"success":false,"message":"Invalid authentication token"}`))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"success":true,"message":"Restart signal sent, application will restart shortly"}`))

	// Trigger restart by cancelling context
	// این کار باعث می‌شود برنامه اصلی Hot Reload را انجام دهد
	if globalCancel != nil {
		go func() {
			time.Sleep(100 * time.Millisecond) // مهلت برای بازگشت پاسخ
			globalCancel()
		}()
	}
}

// verifyRestartToken تابع کمکی برای بررسی token restart
// verifyRestartToken تابع کمکی برای بررسی token restart
func verifyRestartToken(token, configType string) error {
	if configProvider == nil {
		return fmt.Errorf("config provider not available")
	}

	var expectedToken string

	if configType == "server" {
		cfg := configProvider.GetServerConfig()
		if cfg == nil {
			return fmt.Errorf("server config not found")
		}
		expectedToken = cfg.Token

		// اگر token خالی بود، مستقیم از فایل بخوانید
		if expectedToken == "" {
			filePath := configProvider.GetConfigFilePath()
			expectedToken = readTokenFromConfigFile(filePath, "server")
		}
	} else {
		cfg := configProvider.GetClientConfig()
		if cfg == nil {
			return fmt.Errorf("client config not found")
		}
		expectedToken = cfg.Token

		// اگر token خالی بود، مستقیم از فایل بخوانید
		if expectedToken == "" {
			filePath := configProvider.GetConfigFilePath()
			expectedToken = readTokenFromConfigFile(filePath, "client")
		}
	}

	if token == "" {
		return fmt.Errorf("token required")
	}

	if token != expectedToken {
		return fmt.Errorf("invalid token")
	}

	return nil
}

// readTokenFromConfigFile خواندن token از فایل TOML (از config_api.go کپی شده)
func readTokenFromConfigFile(filePath, section string) string {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return ""
	}

	content := string(data)
	lines := strings.Split(content, "\n")

	var inSection bool
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// بررسی اینکه در قسمت صحیح هستیم
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			sectionName := strings.Trim(trimmed, "[]")
			inSection = sectionName == section
			continue
		}

		if inSection && strings.HasPrefix(trimmed, "token") && !strings.HasPrefix(trimmed, "#") {
			parts := strings.Split(trimmed, "=")
			if len(parts) >= 2 {
				value := strings.TrimSpace(parts[1])
				// حذف علامات نقل‌قول و کمان
				value = strings.Trim(value, "\"'")
				return value
			}
		}
	}

	return ""
}
