package web

import (
	"io/ioutil"
	"os"
	"strings"
	"sync"

	"github.com/BurntSushi/toml"
	"github.com/musix/backhaul/internal/config"
)

// DirectFileConfigProvider provides config access directly from file
// This is used when the main service crashes but API needs to work
type DirectFileConfigProvider struct {
	filePath    string
	mu          sync.RWMutex
	cfg         *config.Config
	serverToken string // Cached server token for fallback
	clientToken string // Cached client token for fallback
}

// NewDirectFileConfigProvider creates a new direct file config provider
func NewDirectFileConfigProvider(filePath string) *DirectFileConfigProvider {
	provider := &DirectFileConfigProvider{
		filePath: filePath,
		cfg:      &config.Config{},
	}

	// Try to load config from file
	var cfg config.Config
	if _, err := toml.DecodeFile(filePath, &cfg); err == nil {
		provider.cfg = &cfg
		// Cache tokens if successfully loaded
		if cfg.Server != nil {
			provider.serverToken = cfg.Server.Token
		}
		if cfg.Client != nil {
			provider.clientToken = cfg.Client.Token
		}
	} else {
		// If config loading fails, try to extract tokens and ports with lenient parsing
		provider.cfg = &config.Config{
			Server: &config.ServerConfig{},
			Client: &config.ClientConfig{},
		}
		provider.serverToken = extractTokenFromFile(filePath, "server")
		provider.clientToken = extractTokenFromFile(filePath, "client")

		// Also extract ports from server config
		serverPorts := extractPortsFromFile(filePath, "server")
		if len(serverPorts) > 0 {
			provider.cfg.Server.Ports = serverPorts
		}

		// Set extracted tokens in config
		if provider.serverToken != "" {
			provider.cfg.Server.Token = provider.serverToken
		}
		if provider.clientToken != "" {
			provider.cfg.Client.Token = provider.clientToken
		}
	}

	return provider
}

// GetServerConfig returns server config from file
func (d *DirectFileConfigProvider) GetServerConfig() *config.ServerConfig {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if d.cfg != nil && d.cfg.Server != nil {
		cfg := d.cfg.Server
		// Ensure Ports is initialized
		if cfg.Ports == nil {
			cfg.Ports = make([]string, 0)
		}
		return cfg
	}
	return &config.ServerConfig{}
}

// GetClientConfig returns client config from file
func (d *DirectFileConfigProvider) GetClientConfig() *config.ClientConfig {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if d.cfg != nil && d.cfg.Client != nil {
		return d.cfg.Client
	}
	return &config.ClientConfig{}
}

// GetConfigFilePath returns the config file path
func (d *DirectFileConfigProvider) GetConfigFilePath() string {
	return d.filePath
}

// SaveConfig saves the config to file
func (d *DirectFileConfigProvider) SaveConfig() error {
	// Not used in direct file mode
	return nil
}

// SaveConfigUpdates saves specific config updates to file
func (d *DirectFileConfigProvider) SaveConfigUpdates(updates map[string]interface{}, configType string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Update config in memory
	if configType == "server" && d.cfg.Server != nil {
		for key, value := range updates {
			applyUpdateToServerConfig(d.cfg.Server, key, value)
			// Save each update to file
			if err := updateTOMLValue(d.filePath, configType, key, value); err != nil {
				return err
			}
		}
	} else if configType == "client" && d.cfg.Client != nil {
		for key, value := range updates {
			applyUpdateToClientConfig(d.cfg.Client, key, value)
			// Save each update to file
			if err := updateTOMLValue(d.filePath, configType, key, value); err != nil {
				return err
			}
		}
	}

	return nil
}

// Helper functions to apply updates
func applyUpdateToServerConfig(cfg *config.ServerConfig, key string, value interface{}) {
	switch key {
	case "log_level":
		if s, ok := value.(string); ok {
			cfg.LogLevel = s
		}
	case "nodelay":
		if b, ok := value.(bool); ok {
			cfg.Nodelay = b
		}
	case "keepalive_period":
		if i, ok := value.(int); ok {
			cfg.Keepalive = i
		}
	case "ports":
		if ports, ok := value.([]string); ok {
			cfg.Ports = ports
		}
		// Add more fields as needed
	}
}

func applyUpdateToClientConfig(cfg *config.ClientConfig, key string, value interface{}) {
	switch key {
	case "log_level":
		if s, ok := value.(string); ok {
			cfg.LogLevel = s
		}
	case "nodelay":
		if b, ok := value.(bool); ok {
			cfg.Nodelay = b
		}
	case "keepalive_period":
		if i, ok := value.(int); ok {
			cfg.Keepalive = i
		}
		// Add more fields as needed
	}
}

// extractTokenFromFile attempts to read token directly from TOML file
// Uses lenient line-by-line parsing to extract token even if config has syntax errors
func extractTokenFromFile(filePath string, section string) string {
	content, err := ioutil.ReadFile(filePath)
	if err != nil {
		return ""
	}

	lines := strings.Split(string(content), "\n")
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

		// Skip comments and empty lines
		if strings.HasPrefix(trimmed, "#") || trimmed == "" {
			continue
		}

		// Look for token in this section
		if inSection && strings.HasPrefix(trimmed, "token") {
			// Extract value after "token = "
			if idx := strings.Index(trimmed, "="); idx != -1 {
				value := strings.TrimSpace(trimmed[idx+1:])
				// Remove quotes if present
				value = strings.Trim(value, "\"'")
				if value != "" {
					return value
				}
			}
		}
	}

	return ""
}

// extractPortsFromFile attempts to read ports array from TOML file
// Uses lenient parsing to extract ports even if config has syntax errors
func extractPortsFromFile(filePath string, section string) []string {
	content, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil
	}

	lines := strings.Split(string(content), "\n")
	var ports []string
	var inSection bool
	var inPorts bool

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Check if we're entering the section
		if trimmed == "["+section+"]" {
			inSection = true
			continue
		}

		// If we encounter another section header, stop looking
		if inSection && strings.HasPrefix(trimmed, "[") {
			break
		}

		// Skip comments and empty lines
		if strings.HasPrefix(trimmed, "#") || trimmed == "" {
			continue
		}

		// Look for ports array start in this section
		if inSection && strings.HasPrefix(trimmed, "ports") {
			inPorts = true
			// Check if entire array is on one line like: ports = ["8443", "16445"]
			if strings.Contains(trimmed, "[") && strings.Contains(trimmed, "]") {
				// Parse single-line array
				startIdx := strings.Index(trimmed, "[")
				endIdx := strings.LastIndex(trimmed, "]")
				if startIdx >= 0 && endIdx > startIdx {
					arrayStr := trimmed[startIdx+1 : endIdx]
					parts := strings.Split(arrayStr, ",")
					for _, part := range parts {
						port := strings.TrimSpace(part)
						port = strings.Trim(port, "\"'")
						if port != "" {
							ports = append(ports, port)
						}
					}
				}
			}
			continue
		}

		// If we're in multi-line ports array, parse entries
		if inPorts && inSection {
			if strings.Contains(trimmed, "]") {
				inPorts = false
				continue
			}

			// Parse port entry
			port := strings.Trim(trimmed, "\",")
			port = strings.TrimSpace(port)
			port = strings.Trim(port, "\"'")
			if port != "" {
				ports = append(ports, port)
			}
		}
	}

	return ports
}

// For ports, it does lenient line-by-line replacement to preserve other fields
func updateTOMLValue(filePath string, section string, key string, value interface{}) error {
	if key == "ports" {
		// Use lenient line-by-line parsing for ports to preserve other fields
		return updatePortsInFile(filePath, section, value)
	}

	// For other fields, use standard TOML parsing
	var data map[string]interface{}
	if _, err := toml.DecodeFile(filePath, &data); err != nil {
		// If file can't be parsed, create new data
		data = make(map[string]interface{})
	}

	// Ensure the section exists
	if _, exists := data[section]; !exists {
		data[section] = make(map[string]interface{})
	}

	// Get the section as a map
	sectionMap := data[section].(map[string]interface{})
	if sectionMap == nil {
		sectionMap = make(map[string]interface{})
		data[section] = sectionMap
	}

	// Update the value
	sectionMap[key] = value

	// Write back to file
	f, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	encoder := toml.NewEncoder(f)
	return encoder.Encode(data)
}

// updatePortsInFile updates ports array while preserving all other config
func updatePortsInFile(filePath string, section string, value interface{}) error {
	// Read existing file content
	content, err := ioutil.ReadFile(filePath)
	if err != nil {
		// If file doesn't exist, create it
		return writePortsToNewFile(filePath, section, value)
	}

	lines := strings.Split(string(content), "\n")
	var output []string
	var inSection bool
	var inPorts bool
	var portStartLine = -1
	var portEndLine = -1

	// First pass: identify ports section
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Check if we're entering the target section
		if trimmed == "["+section+"]" {
			inSection = true
		} else if inSection && strings.HasPrefix(trimmed, "[") && trimmed != "["+section+"]" {
			// We've left the target section
			inSection = false
		}

		// If in section and we find ports array start
		if inSection && strings.HasPrefix(trimmed, "ports") && strings.Contains(trimmed, "[") {
			inPorts = true
			portStartLine = i
		}

		// If we're in ports array, look for closing bracket
		if inPorts && (strings.Contains(line, "]")) {
			portEndLine = i
			inPorts = false
			break
		}
	}

	// Second pass: build output, skipping old ports
	inSection = false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Check if entering/leaving target section
		if trimmed == "["+section+"]" {
			inSection = true
		} else if inSection && strings.HasPrefix(trimmed, "[") {
			inSection = false
		}

		// Skip old ports lines
		if portStartLine >= 0 && i >= portStartLine && i <= portEndLine {
			if i == portStartLine {
				// Insert new ports at this position
				output = append(output, formatPortsArray(value))
			}
			continue
		}

		output = append(output, line)
	}

	// If ports section wasn't found, append it at end of section
	if portStartLine == -1 && inSection {
		// Find where section ends
		for i := len(output) - 1; i >= 0; i-- {
			if trimmed := strings.TrimSpace(output[i]); trimmed != "" && !strings.HasPrefix(trimmed, "[") {
				output = append(output[:i+1], append([]string{formatPortsArray(value)}, output[i+1:]...)...)
				break
			}
		}
	}

	// Write back to file
	f, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.WriteString(strings.Join(output, "\n"))
	return err
}

// formatPortsArray formats ports array for TOML file
func formatPortsArray(value interface{}) string {
	var ports []string

	switch v := value.(type) {
	case []interface{}:
		for _, port := range v {
			if s, ok := port.(string); ok {
				ports = append(ports, s)
			}
		}
	case []string:
		ports = v
	}

	if len(ports) == 0 {
		return "ports = []"
	}

	result := "ports = [\n"
	for i, port := range ports {
		result += "\t\"" + port + "\""
		if i < len(ports)-1 {
			result += ","
		}
		result += "\n"
	}
	result += "]"
	return result
}

// writePortsToNewFile creates a new TOML file with ports
func writePortsToNewFile(filePath string, section string, value interface{}) error {
	f, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	f.WriteString("[" + section + "]\n")
	f.WriteString(formatPortsArray(value) + "\n")
	return nil
}
