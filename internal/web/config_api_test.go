package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/musix/backhaul/internal/config"
)

type mockConfigProvider struct {
	serverConfig   *config.ServerConfig
	clientConfig   *config.ClientConfig
	configFilePath string
}

func (m *mockConfigProvider) GetServerConfig() *config.ServerConfig {
	return m.serverConfig
}

func (m *mockConfigProvider) GetClientConfig() *config.ClientConfig {
	return m.clientConfig
}

func (m *mockConfigProvider) GetConfigFilePath() string {
	return m.configFilePath
}

func (m *mockConfigProvider) SaveConfig() error {
	return nil
}

func (m *mockConfigProvider) SaveConfigUpdates(updates map[string]interface{}, configType string) error {
	return nil
}

// TestHandleConfigUpdateValidation tests validation of config updates
func TestHandleConfigUpdateValidation(t *testing.T) {
	tests := []struct {
		name           string
		configType     string
		update         ConfigUpdate
		expectedErrors int
		shouldStatusOK bool
		description    string
	}{
		{
			name:           "Valid Server Config Update - Log Level",
			configType:     "server",
			update:         ConfigUpdate{Type: "server", LogLevel: "debug"},
			expectedErrors: 0,
			shouldStatusOK: true,
			description:    "Should accept valid log level",
		},
		{
			name:           "Invalid Log Level",
			configType:     "server",
			update:         ConfigUpdate{Type: "server", LogLevel: "invalid"},
			expectedErrors: 1,
			shouldStatusOK: false,
			description:    "Should reject invalid log level",
		},
		{
			name:           "Invalid Type",
			configType:     "",
			update:         ConfigUpdate{Type: "invalid"},
			expectedErrors: 1,
			shouldStatusOK: false,
			description:    "Should reject invalid config type",
		},
		{
			name:           "Negative Keepalive",
			configType:     "server",
			update:         ConfigUpdate{Type: "server", Keepalive: intPtr(-10)},
			expectedErrors: 1,
			shouldStatusOK: false,
			description:    "Should reject negative keepalive",
		},
		{
			name:           "Invalid Mux Version",
			configType:     "server",
			update:         ConfigUpdate{Type: "server", MuxVersion: intPtr(3)},
			expectedErrors: 1,
			shouldStatusOK: false,
			description:    "Should reject invalid mux version",
		},
		{
			name:           "Valid Mux Config",
			configType:     "server",
			update:         ConfigUpdate{Type: "server", MuxVersion: intPtr(2), MaxFrameSize: intPtr(65536)},
			expectedErrors: 0,
			shouldStatusOK: true,
			description:    "Should accept valid mux configuration",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors := ValidateConfigUpdate(&tt.update)
			if len(errors) != tt.expectedErrors {
				t.Errorf("Expected %d validation errors, got %d: %v", tt.expectedErrors, len(errors), errors)
			}

			if len(errors) > 0 && tt.shouldStatusOK {
				t.Errorf("Expected valid config but got errors: %v", errors)
			}
		})
	}
}

// TestHandleConfigUpdateEndpoint tests the HTTP endpoint
func TestHandleConfigUpdateEndpoint(t *testing.T) {
	// Setup mock config provider
	mockProvider := &mockConfigProvider{
		serverConfig: &config.ServerConfig{
			LogLevel:  "info",
			Nodelay:   false,
			Keepalive: 30,
			Token:     "test-secret-token",
		},
		clientConfig: &config.ClientConfig{
			LogLevel:      "info",
			RetryInterval: 5,
			Token:         "client-secret-token",
		},
	}

	configProvider = mockProvider

	tests := []struct {
		name           string
		method         string
		body           ConfigUpdate
		expectedStatus int
		description    string
	}{
		{
			name:           "POST Valid Server Config with Token",
			method:         "POST",
			body:           ConfigUpdate{Type: "server", Token: "test-secret-token", LogLevel: "debug"},
			expectedStatus: http.StatusOK,
			description:    "Should update server config successfully with valid token",
		},
		{
			name:           "POST Server Config with Invalid Token",
			method:         "POST",
			body:           ConfigUpdate{Type: "server", Token: "wrong-token", LogLevel: "debug"},
			expectedStatus: http.StatusUnauthorized,
			description:    "Should reject invalid token",
		},
		{
			name:           "POST Server Config without Token",
			method:         "POST",
			body:           ConfigUpdate{Type: "server", LogLevel: "debug"},
			expectedStatus: http.StatusUnauthorized,
			description:    "Should reject missing token",
		},
		{
			name:           "PUT Valid Client Config with Token",
			method:         "PUT",
			body:           ConfigUpdate{Type: "client", Token: "client-secret-token", LogLevel: "warn"},
			expectedStatus: http.StatusOK,
			description:    "Should update client config successfully with valid token",
		},
		{
			name:           "Invalid Method GET",
			method:         "GET",
			body:           ConfigUpdate{},
			expectedStatus: http.StatusMethodNotAllowed,
			description:    "Should reject GET method",
		},
		{
			name:           "Invalid Validation",
			method:         "POST",
			body:           ConfigUpdate{Type: "server", Token: "test-secret-token", LogLevel: "invalid"},
			expectedStatus: http.StatusBadRequest,
			description:    "Should reject invalid log level",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(tt.method, "/api/config", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")

			w := httptest.NewRecorder()
			HandleConfigUpdate(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d: %s", tt.expectedStatus, w.Code, w.Body.String())
			}
		})
	}
}

// TestLogLevelValidation tests log level validation
func TestLogLevelValidation(t *testing.T) {
	validLevels := []string{"trace", "debug", "info", "warn", "error", "fatal", "TRACE", "DEBUG", "INFO"}
	for _, level := range validLevels {
		if !isValidLogLevel(level) {
			t.Errorf("Expected %s to be valid", level)
		}
	}

	invalidLevels := []string{"invalid", "silly", "verbose", ""}
	for _, level := range invalidLevels {
		if isValidLogLevel(level) {
			t.Errorf("Expected %s to be invalid", level)
		}
	}
}

// TestServerConfigUpdate tests server config update logic
func TestServerConfigUpdate(t *testing.T) {
	snifferVal := false
	cfg := &config.ServerConfig{
		LogLevel:     "info",
		Nodelay:      false,
		Keepalive:    30,
		MuxSession:   1,
		MuxVersion:   1,
		MaxFrameSize: 32768,
		ChannelSize:  1024,
		Sniffer:      &snifferVal,
		Heartbeat:    30,
		AcceptUDP:    false,
		TLSCertFile:  "",
		TLSKeyFile:   "",
	}

	update := &ConfigUpdate{
		Type:       "server",
		LogLevel:   "debug",
		Nodelay:    boolPtr(true),
		Keepalive:  intPtr(60),
		MuxSession: intPtr(2),
	}

	changes := applyServerConfigUpdates(cfg, update)

	// Verify changes were applied
	if cfg.LogLevel != "debug" {
		t.Errorf("Expected log_level to be 'debug', got '%s'", cfg.LogLevel)
	}

	if *boolPtr(cfg.Nodelay) != true {
		t.Errorf("Expected nodelay to be true, got %v", cfg.Nodelay)
	}

	if cfg.Keepalive != 60 {
		t.Errorf("Expected keepalive to be 60, got %d", cfg.Keepalive)
	}

	if cfg.MuxSession != 2 {
		t.Errorf("Expected mux_session to be 2, got %d", cfg.MuxSession)
	}

	// Verify changes were tracked
	if len(changes) != 4 {
		t.Errorf("Expected 4 changes tracked, got %d", len(changes))
	}
}

// TestClientConfigUpdate tests client config update logic
func TestClientConfigUpdate(t *testing.T) {
	cfg := &config.ClientConfig{
		LogLevel:      "info",
		Nodelay:       false,
		Keepalive:     30,
		RetryInterval: 5,
		DialTimeout:   10,
	}

	update := &ConfigUpdate{
		Type:          "client",
		LogLevel:      "debug",
		RetryInterval: intPtr(10),
		DialTimeout:   intPtr(20),
	}

	changes := applyClientConfigUpdates(cfg, update)

	if cfg.LogLevel != "debug" {
		t.Errorf("Expected log_level to be 'debug', got '%s'", cfg.LogLevel)
	}

	if cfg.RetryInterval != 10 {
		t.Errorf("Expected retry_interval to be 10, got %d", cfg.RetryInterval)
	}

	if cfg.DialTimeout != 20 {
		t.Errorf("Expected dial_timeout to be 20, got %d", cfg.DialTimeout)
	}

	if len(changes) != 3 {
		t.Errorf("Expected 3 changes tracked, got %d", len(changes))
	}
}

// TestVerifyAuthToken tests token verification
func TestVerifyAuthToken(t *testing.T) {
	mockProvider := &mockConfigProvider{
		serverConfig: &config.ServerConfig{
			Token: "server-token-123",
		},
		clientConfig: &config.ClientConfig{
			Token: "client-token-456",
		},
	}

	configProvider = mockProvider

	tests := []struct {
		name          string
		token         string
		update        ConfigUpdate
		shouldSucceed bool
		description   string
	}{
		{
			name:          "Valid Server Token",
			token:         "server-token-123",
			update:        ConfigUpdate{Type: "server"},
			shouldSucceed: true,
			description:   "Should accept valid server token",
		},
		{
			name:          "Invalid Server Token",
			token:         "wrong-token",
			update:        ConfigUpdate{Type: "server"},
			shouldSucceed: false,
			description:   "Should reject invalid server token",
		},
		{
			name:          "Empty Token",
			token:         "",
			update:        ConfigUpdate{Type: "server"},
			shouldSucceed: false,
			description:   "Should reject empty token",
		},
		{
			name:          "Valid Client Token",
			token:         "client-token-456",
			update:        ConfigUpdate{Type: "client"},
			shouldSucceed: true,
			description:   "Should accept valid client token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := VerifyAuthToken(tt.token, &tt.update)
			if tt.shouldSucceed && err != nil {
				t.Errorf("Expected token verification to succeed, but got error: %v", err)
			}
			if !tt.shouldSucceed && err == nil {
				t.Errorf("Expected token verification to fail, but it succeeded")
			}
		})
	}
}

// Helper functions
func intPtr(i int) *int {
	return &i
}

func boolPtr(b bool) *bool {
	return &b
}
