package web

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"sync"

	"github.com/musix/backhaul/internal/config"
	"github.com/sirupsen/logrus"
)

// ConfigUpdate represents the structure for updating configuration
type ConfigUpdate struct {
	Type             string   `json:"type"`  // "server" or "client"
	Token            string   `json:"token"` // Authentication token
	LogLevel         string   `json:"log_level,omitempty"`
	Nodelay          *bool    `json:"nodelay,omitempty"`
	Keepalive        *int     `json:"keepalive_period,omitempty"`
	PPROF            *bool    `json:"pprof,omitempty"`
	MuxSession       *int     `json:"mux_session,omitempty"`
	MuxVersion       *int     `json:"mux_version,omitempty"`
	MaxFrameSize     *int     `json:"mux_framesize,omitempty"`
	MaxReceiveBuffer *int     `json:"mux_recievebuffer,omitempty"`
	MaxStreamBuffer  *int     `json:"mux_streambuffer,omitempty"`
	Sniffer          *bool    `json:"sniffer,omitempty"`
	WebPort          *int     `json:"web_port,omitempty"`
	Heartbeat        *int     `json:"heartbeat,omitempty"`
	MuxCon           *int     `json:"mux_con,omitempty"`
	AcceptUDP        *bool    `json:"accept_udp,omitempty"`
	ChannelSize      *int     `json:"channel_size,omitempty"`
	RetryInterval    *int     `json:"retry_interval,omitempty"`
	DialTimeout      *int     `json:"dial_timeout,omitempty"`
	AggressivePool   *bool    `json:"aggressive_pool,omitempty"`
	Ports            []string `json:"ports,omitempty"`
	TLSCertFile      string   `json:"tls_cert,omitempty"`
	TLSKeyFile       string   `json:"tls_key,omitempty"`
}

type ConfigResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// ConfigUpdateMutex ensures thread-safe config updates
var configUpdateMutex sync.RWMutex

// ValidateConfigUpdate validates the config update request
func ValidateConfigUpdate(update *ConfigUpdate) []ValidationError {
	var errors []ValidationError

	if update.Type != "server" && update.Type != "client" {
		errors = append(errors, ValidationError{
			Field:   "type",
			Message: "type must be 'server' or 'client'",
		})
	}

	if update.LogLevel != "" && !isValidLogLevel(update.LogLevel) {
		errors = append(errors, ValidationError{
			Field:   "log_level",
			Message: "invalid log level: must be one of trace, debug, info, warn, error, fatal",
		})
	}

	if update.Keepalive != nil && *update.Keepalive < 0 {
		errors = append(errors, ValidationError{
			Field:   "keepalive_period",
			Message: "keepalive_period must be non-negative",
		})
	}

	if update.MuxSession != nil && *update.MuxSession < 1 {
		errors = append(errors, ValidationError{
			Field:   "mux_session",
			Message: "mux_session must be at least 1",
		})
	}

	if update.MuxVersion != nil && (*update.MuxVersion < 1 || *update.MuxVersion > 2) {
		errors = append(errors, ValidationError{
			Field:   "mux_version",
			Message: "mux_version must be 1 or 2",
		})
	}

	if update.MaxFrameSize != nil && *update.MaxFrameSize < 1024 {
		errors = append(errors, ValidationError{
			Field:   "mux_framesize",
			Message: "mux_framesize must be at least 1024",
		})
	}

	if update.Heartbeat != nil && *update.Heartbeat < 1 {
		errors = append(errors, ValidationError{
			Field:   "heartbeat",
			Message: "heartbeat must be at least 1 second",
		})
	}

	if update.WebPort != nil && (*update.WebPort < 0 || *update.WebPort > 65535) {
		errors = append(errors, ValidationError{
			Field:   "web_port",
			Message: "web_port must be between 0 and 65535",
		})
	}

	if update.ChannelSize != nil && *update.ChannelSize < 1 {
		errors = append(errors, ValidationError{
			Field:   "channel_size",
			Message: "channel_size must be at least 1",
		})
	}

	return errors
}

// isValidLogLevel checks if the log level is valid
func isValidLogLevel(level string) bool {
	validLevels := []string{"trace", "debug", "info", "warn", "error", "fatal"}
	for _, v := range validLevels {
		if strings.EqualFold(v, level) {
			return true
		}
	}
	return false
}

// VerifyAuthToken verifies the authentication token against config
func VerifyAuthToken(token string, update *ConfigUpdate) error {
	if configProvider == nil {
		return &AuthError{
			Code:    "PROVIDER_NOT_SET",
			Message: "Config provider not set",
		}
	}

	var expectedToken string

	if update.Type == "server" {
		cfg := configProvider.GetServerConfig()
		if cfg == nil {
			return &AuthError{
				Code:    "CONFIG_NOT_FOUND",
				Message: "Server config not found",
			}
		}
		expectedToken = cfg.Token

		// If token is empty, try to read directly from file as fallback
		if expectedToken == "" {
			filePath := configProvider.GetConfigFilePath()
			expectedToken = readTokenFromFile(filePath, "server")
		}
	} else {
		cfg := configProvider.GetClientConfig()
		if cfg == nil {
			return &AuthError{
				Code:    "CONFIG_NOT_FOUND",
				Message: "Client config not found",
			}
		}
		expectedToken = cfg.Token

		// If token is empty, try to read directly from file as fallback
		if expectedToken == "" {
			filePath := configProvider.GetConfigFilePath()
			expectedToken = readTokenFromFile(filePath, "client")
		}
	}

	if token == "" {
		return &AuthError{
			Code:    "TOKEN_MISSING",
			Message: "Authentication token is required",
		}
	}

	if token != expectedToken {
		logrus.WithFields(logrus.Fields{
			"config_type": update.Type,
		}).Warn("[WEB] Invalid authentication token attempted")
		return &AuthError{
			Code:    "TOKEN_INVALID",
			Message: "Invalid authentication token",
		}
	}

	return nil
}

// readTokenFromFile attempts to read token directly from TOML file
// Uses a lenient approach to extract token even if config has parse errors
func readTokenFromFile(filePath string, section string) string {
	data, err := readFileContent(filePath)
	if err != nil {
		return ""
	}

	// Try to find token value with lenient parsing
	// Look for "token = " pattern in the section
	lines := strings.Split(data, "\n")
	inSection := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Check if we're entering the section
		if strings.HasPrefix(trimmed, "["+section+"]") {
			inSection = true
			continue
		}

		// If we encounter another section header, stop looking
		if inSection && strings.HasPrefix(trimmed, "[") {
			break
		}

		// Look for token in this section
		if inSection && strings.HasPrefix(trimmed, "token") {
			// Extract value after "token = "
			if idx := strings.Index(trimmed, "="); idx != -1 {
				value := strings.TrimSpace(trimmed[idx+1:])
				// Remove quotes if present
				value = strings.Trim(value, "\"'")
				return value
			}
		}
	}

	return ""
}

// readFileContent reads the raw file content
func readFileContent(filePath string) (string, error) {
	content, err := ioutil.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	return string(content), nil
} // AuthError represents an authentication error
type AuthError struct {
	Code    string
	Message string
}

func (e *AuthError) Error() string {
	return e.Message
}

// convertChangesToTOML converts applied changes to TOML values for file saving
func convertChangesToTOML(appliedChanges map[string]interface{}) map[string]interface{} {
	tomlValues := make(map[string]interface{})
	for key, changeInfo := range appliedChanges {
		if change, ok := changeInfo.(map[string]interface{}); ok {
			if newVal, exists := change["new"]; exists {
				tomlValues[key] = newVal
			}
		}
	}
	return tomlValues
}

// HandleConfigUpdate handles POST requests to update config
func HandleConfigUpdate(w http.ResponseWriter, r *http.Request) {
	// Recover from panic to ensure API stays up
	defer func() {
		if recovery := recover(); recovery != nil {
			logrus.WithFields(logrus.Fields{
				"panic": fmt.Sprintf("%v", recovery),
			}).Error("[WEB] Config update handler panic recovered")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(ConfigResponse{
				Success: false,
				Message: "Internal server error (recovered from panic)",
			})
		}
	}()

	if r.Method != http.MethodPost && r.Method != http.MethodPut {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ConfigResponse{
			Success: false,
			Message: "Only POST and PUT methods are supported",
		})
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if configProvider == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ConfigResponse{
			Success: false,
			Message: "Config provider not set",
		})
		return
	}

	var update ConfigUpdate
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ConfigResponse{
			Success: false,
			Message: "Invalid JSON request: " + err.Error(),
		})
		return
	}

	// Verify authentication token first
	if authErr := VerifyAuthToken(update.Token, &update); authErr != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(ConfigResponse{
			Success: false,
			Message: authErr.Error(),
		})
		return
	}

	// Validate the update
	validationErrors := ValidateConfigUpdate(&update)
	if len(validationErrors) > 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ConfigResponse{
			Success: false,
			Message: "Validation errors",
			Data:    validationErrors,
		})
		return
	}

	// Apply the update
	configUpdateMutex.Lock()
	defer configUpdateMutex.Unlock()

	if update.Type == "server" {
		cfg := configProvider.GetServerConfig()
		if cfg == nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(ConfigResponse{
				Success: false,
				Message: "Server config not found",
			})
			return
		}

		appliedChanges := applyServerConfigUpdates(cfg, &update)

		logrus.WithFields(logrus.Fields{
			"applied_changes": fmt.Sprintf("%v", appliedChanges),
			"config_type":     update.Type,
		}).Debug("[WEB] Applied changes to server config")

		// Save only the changed fields to file
		tomlValues := convertChangesToTOML(appliedChanges)
		logrus.WithFields(logrus.Fields{
			"toml_values": fmt.Sprintf("%v", tomlValues),
			"config_type": update.Type,
		}).Debug("[WEB] TOML values to save")

		if err := configProvider.SaveConfigUpdates(tomlValues, "server"); err != nil {
			logrus.WithFields(logrus.Fields{
				"config_type": update.Type,
				"error":       err.Error(),
			}).Error("[WEB] Failed to save config to file")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(ConfigResponse{
				Success: false,
				Message: "Configuration updated in memory but failed to save to file: " + err.Error(),
			})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(ConfigResponse{
			Success: true,
			Message: "Server configuration updated successfully",
			Data: map[string]interface{}{
				"appliedChanges": appliedChanges,
			},
		})
	} else {
		cfg := configProvider.GetClientConfig()
		if cfg == nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(ConfigResponse{
				Success: false,
				Message: "Client config not found",
			})
			return
		}

		appliedChanges := applyClientConfigUpdates(cfg, &update)

		logrus.WithFields(logrus.Fields{
			"applied_changes": fmt.Sprintf("%v", appliedChanges),
			"config_type":     update.Type,
		}).Debug("[WEB] Applied changes to client config")

		// Save only the changed fields to file
		tomlValues := convertChangesToTOML(appliedChanges)
		logrus.WithFields(logrus.Fields{
			"toml_values": fmt.Sprintf("%v", tomlValues),
			"config_type": update.Type,
		}).Debug("[WEB] TOML values to save")

		if err := configProvider.SaveConfigUpdates(tomlValues, "client"); err != nil {
			logrus.WithFields(logrus.Fields{
				"config_type": update.Type,
				"error":       err.Error(),
			}).Error("[WEB] Failed to save config to file")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(ConfigResponse{
				Success: false,
				Message: "Configuration updated in memory but failed to save to file: " + err.Error(),
			})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(ConfigResponse{
			Success: true,
			Message: "Client configuration updated successfully",
			Data: map[string]interface{}{
				"appliedChanges": appliedChanges,
			},
		})
	}
}

// applyServerConfigUpdates applies updates to server config
func applyServerConfigUpdates(cfg *config.ServerConfig, update *ConfigUpdate) map[string]interface{} {
	appliedChanges := make(map[string]interface{})

	if update.LogLevel != "" && update.LogLevel != cfg.LogLevel {
		appliedChanges["log_level"] = map[string]interface{}{
			"old": cfg.LogLevel,
			"new": update.LogLevel,
		}
		cfg.LogLevel = update.LogLevel
	} else if update.LogLevel != "" && update.LogLevel == cfg.LogLevel {
		// Even if no change, add it to appliedChanges so it gets written to file
		appliedChanges["log_level"] = map[string]interface{}{
			"old": cfg.LogLevel,
			"new": update.LogLevel,
		}
	}

	if update.Nodelay != nil && *update.Nodelay != cfg.Nodelay {
		appliedChanges["nodelay"] = map[string]interface{}{
			"old": cfg.Nodelay,
			"new": *update.Nodelay,
		}
		cfg.Nodelay = *update.Nodelay
	}

	if update.Keepalive != nil && *update.Keepalive != cfg.Keepalive {
		appliedChanges["keepalive_period"] = map[string]interface{}{
			"old": cfg.Keepalive,
			"new": *update.Keepalive,
		}
		cfg.Keepalive = *update.Keepalive
	}

	if update.PPROF != nil && *update.PPROF != cfg.PPROF {
		appliedChanges["pprof"] = map[string]interface{}{
			"old": cfg.PPROF,
			"new": *update.PPROF,
		}
		cfg.PPROF = *update.PPROF
	}

	if update.MuxSession != nil && *update.MuxSession != cfg.MuxSession {
		appliedChanges["mux_session"] = map[string]interface{}{
			"old": cfg.MuxSession,
			"new": *update.MuxSession,
		}
		cfg.MuxSession = *update.MuxSession
	}

	if update.MuxVersion != nil && *update.MuxVersion != cfg.MuxVersion {
		appliedChanges["mux_version"] = map[string]interface{}{
			"old": cfg.MuxVersion,
			"new": *update.MuxVersion,
		}
		cfg.MuxVersion = *update.MuxVersion
	}

	if update.MaxFrameSize != nil && *update.MaxFrameSize != cfg.MaxFrameSize {
		appliedChanges["mux_framesize"] = map[string]interface{}{
			"old": cfg.MaxFrameSize,
			"new": *update.MaxFrameSize,
		}
		cfg.MaxFrameSize = *update.MaxFrameSize
	}

	if update.MaxReceiveBuffer != nil && *update.MaxReceiveBuffer != cfg.MaxReceiveBuffer {
		appliedChanges["mux_recievebuffer"] = map[string]interface{}{
			"old": cfg.MaxReceiveBuffer,
			"new": *update.MaxReceiveBuffer,
		}
		cfg.MaxReceiveBuffer = *update.MaxReceiveBuffer
	}

	if update.MaxStreamBuffer != nil && *update.MaxStreamBuffer != cfg.MaxStreamBuffer {
		appliedChanges["mux_streambuffer"] = map[string]interface{}{
			"old": cfg.MaxStreamBuffer,
			"new": *update.MaxStreamBuffer,
		}
		cfg.MaxStreamBuffer = *update.MaxStreamBuffer
	}

	if update.Sniffer != nil && *update.Sniffer != *cfg.Sniffer {
		appliedChanges["sniffer"] = map[string]interface{}{
			"old": *cfg.Sniffer,
			"new": *update.Sniffer,
		}
		*cfg.Sniffer = *update.Sniffer
	}

	if update.Heartbeat != nil && *update.Heartbeat != cfg.Heartbeat {
		appliedChanges["heartbeat"] = map[string]interface{}{
			"old": cfg.Heartbeat,
			"new": *update.Heartbeat,
		}
		cfg.Heartbeat = *update.Heartbeat
	}

	if update.MuxCon != nil && *update.MuxCon != cfg.MuxCon {
		appliedChanges["mux_con"] = map[string]interface{}{
			"old": cfg.MuxCon,
			"new": *update.MuxCon,
		}
		cfg.MuxCon = *update.MuxCon
	}

	if update.AcceptUDP != nil && *update.AcceptUDP != cfg.AcceptUDP {
		appliedChanges["accept_udp"] = map[string]interface{}{
			"old": cfg.AcceptUDP,
			"new": *update.AcceptUDP,
		}
		cfg.AcceptUDP = *update.AcceptUDP
	}

	if update.ChannelSize != nil && *update.ChannelSize != cfg.ChannelSize {
		appliedChanges["channel_size"] = map[string]interface{}{
			"old": cfg.ChannelSize,
			"new": *update.ChannelSize,
		}
		cfg.ChannelSize = *update.ChannelSize
	}

	if len(update.Ports) > 0 {
		oldPorts := cfg.Ports
		// Ensure oldPorts is not nil for JSON response
		if oldPorts == nil {
			oldPorts = make([]string, 0)
		}
		appliedChanges["ports"] = map[string]interface{}{
			"old": oldPorts,
			"new": update.Ports,
		}
		cfg.Ports = update.Ports
	}

	if update.TLSCertFile != "" && update.TLSCertFile != cfg.TLSCertFile {
		appliedChanges["tls_cert"] = map[string]interface{}{
			"old": cfg.TLSCertFile,
			"new": update.TLSCertFile,
		}
		cfg.TLSCertFile = update.TLSCertFile
	}

	if update.TLSKeyFile != "" && update.TLSKeyFile != cfg.TLSKeyFile {
		appliedChanges["tls_key"] = map[string]interface{}{
			"old": cfg.TLSKeyFile,
			"new": update.TLSKeyFile,
		}
		cfg.TLSKeyFile = update.TLSKeyFile
	}

	logrus.WithFields(logrus.Fields{
		"applied_changes": appliedChanges,
		"config_type":     "server",
	}).Info("[WEB] Server configuration updated")

	return appliedChanges
}

// applyClientConfigUpdates applies updates to client config
func applyClientConfigUpdates(cfg *config.ClientConfig, update *ConfigUpdate) map[string]interface{} {
	appliedChanges := make(map[string]interface{})

	if update.LogLevel != "" && update.LogLevel != cfg.LogLevel {
		appliedChanges["log_level"] = map[string]interface{}{
			"old": cfg.LogLevel,
			"new": update.LogLevel,
		}
		cfg.LogLevel = update.LogLevel
	} else if update.LogLevel != "" && update.LogLevel == cfg.LogLevel {
		// Even if no change, add it to appliedChanges so it gets written to file
		appliedChanges["log_level"] = map[string]interface{}{
			"old": cfg.LogLevel,
			"new": update.LogLevel,
		}
	}

	if update.Nodelay != nil && *update.Nodelay != cfg.Nodelay {
		appliedChanges["nodelay"] = map[string]interface{}{
			"old": cfg.Nodelay,
			"new": *update.Nodelay,
		}
		cfg.Nodelay = *update.Nodelay
	}

	if update.Keepalive != nil && *update.Keepalive != cfg.Keepalive {
		appliedChanges["keepalive_period"] = map[string]interface{}{
			"old": cfg.Keepalive,
			"new": *update.Keepalive,
		}
		cfg.Keepalive = *update.Keepalive
	}

	if update.PPROF != nil && *update.PPROF != cfg.PPROF {
		appliedChanges["pprof"] = map[string]interface{}{
			"old": cfg.PPROF,
			"new": *update.PPROF,
		}
		cfg.PPROF = *update.PPROF
	}

	if update.MuxSession != nil && *update.MuxSession != cfg.MuxSession {
		appliedChanges["mux_session"] = map[string]interface{}{
			"old": cfg.MuxSession,
			"new": *update.MuxSession,
		}
		cfg.MuxSession = *update.MuxSession
	}

	if update.MuxVersion != nil && *update.MuxVersion != cfg.MuxVersion {
		appliedChanges["mux_version"] = map[string]interface{}{
			"old": cfg.MuxVersion,
			"new": *update.MuxVersion,
		}
		cfg.MuxVersion = *update.MuxVersion
	}

	if update.MaxFrameSize != nil && *update.MaxFrameSize != cfg.MaxFrameSize {
		appliedChanges["mux_framesize"] = map[string]interface{}{
			"old": cfg.MaxFrameSize,
			"new": *update.MaxFrameSize,
		}
		cfg.MaxFrameSize = *update.MaxFrameSize
	}

	if update.MaxReceiveBuffer != nil && *update.MaxReceiveBuffer != cfg.MaxReceiveBuffer {
		appliedChanges["mux_recievebuffer"] = map[string]interface{}{
			"old": cfg.MaxReceiveBuffer,
			"new": *update.MaxReceiveBuffer,
		}
		cfg.MaxReceiveBuffer = *update.MaxReceiveBuffer
	}

	if update.MaxStreamBuffer != nil && *update.MaxStreamBuffer != cfg.MaxStreamBuffer {
		appliedChanges["mux_streambuffer"] = map[string]interface{}{
			"old": cfg.MaxStreamBuffer,
			"new": *update.MaxStreamBuffer,
		}
		cfg.MaxStreamBuffer = *update.MaxStreamBuffer
	}

	if update.Sniffer != nil && *update.Sniffer != *cfg.Sniffer {
		appliedChanges["sniffer"] = map[string]interface{}{
			"old": *cfg.Sniffer,
			"new": *update.Sniffer,
		}
		*cfg.Sniffer = *update.Sniffer
	}

	if update.RetryInterval != nil && *update.RetryInterval != cfg.RetryInterval {
		appliedChanges["retry_interval"] = map[string]interface{}{
			"old": cfg.RetryInterval,
			"new": *update.RetryInterval,
		}
		cfg.RetryInterval = *update.RetryInterval
	}

	if update.DialTimeout != nil && *update.DialTimeout != cfg.DialTimeout {
		appliedChanges["dial_timeout"] = map[string]interface{}{
			"old": cfg.DialTimeout,
			"new": *update.DialTimeout,
		}
		cfg.DialTimeout = *update.DialTimeout
	}

	if update.AggressivePool != nil && *update.AggressivePool != cfg.AggressivePool {
		appliedChanges["aggressive_pool"] = map[string]interface{}{
			"old": cfg.AggressivePool,
			"new": *update.AggressivePool,
		}
		cfg.AggressivePool = *update.AggressivePool
	}

	if update.ChannelSize != nil && *update.ChannelSize != cfg.ConnectionPool {
		appliedChanges["connection_pool"] = map[string]interface{}{
			"old": cfg.ConnectionPool,
			"new": *update.ChannelSize,
		}
		cfg.ConnectionPool = *update.ChannelSize
	}

	logrus.WithFields(logrus.Fields{
		"applied_changes": appliedChanges,
		"config_type":     "client",
	}).Info("[WEB] Client configuration updated")

	return appliedChanges
}

// Backup management for config rollback
var (
	configBackupMutex sync.Mutex
	configBackups     = make(map[string][]byte) // filePath -> backup content
)

// getConfigBackup retrieves stored backup
func getConfigBackup(filePath string) []byte {
	configBackupMutex.Lock()
	defer configBackupMutex.Unlock()
	if backup, exists := configBackups[filePath]; exists {
		return append([]byte(nil), backup...) // Copy the backup
	}
	return nil
}

// RollbackConfig rolls back config to the last known good state
func RollbackConfig(configPath string) error {
	backup := getConfigBackup(configPath)
	if len(backup) == 0 {
		return fmt.Errorf("no backup available for rollback")
	}

	configUpdateMutex.Lock()
	defer configUpdateMutex.Unlock()

	// Write backup back to file
	err := ioutil.WriteFile(configPath, backup, 0644)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"file_path": configPath,
			"error":     err.Error(),
		}).Error("[WEB] Failed to rollback config file")
		return err
	}

	logrus.WithFields(logrus.Fields{
		"file_path": configPath,
	}).Warn("[WEB] Config rolled back to last known good state")

	// Reload config provider with restored config
	if configProvider != nil {
		// Reload the provider - this will reload from the rolled-back file
		newProvider := NewDirectFileConfigProvider(configPath)
		SetConfigProvider(newProvider)
	}

	return nil
}

// HandleConfigRollback handles GET/POST requests to rollback config
func HandleConfigRollback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(ConfigResponse{
			Success: false,
			Message: "Method not allowed",
		})
		return
	}

	if configProvider == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ConfigResponse{
			Success: false,
			Message: "Config provider not set",
		})
		return
	}

	configFilePath := configProvider.GetConfigFilePath()
	if configFilePath == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ConfigResponse{
			Success: false,
			Message: "Config file path not found",
		})
		return
	}

	// Perform rollback
	err := RollbackConfig(configFilePath)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ConfigResponse{
			Success: false,
			Message: "Rollback failed: " + err.Error(),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(ConfigResponse{
		Success: true,
		Message: "Configuration rolled back successfully to last known good state",
	})
}
