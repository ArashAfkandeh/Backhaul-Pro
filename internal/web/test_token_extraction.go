package web

import (
	"io/ioutil"
	"os"
	"testing"
)

func TestExtractTokenFromMalformedConfig(t *testing.T) {
	// Create a test config file with a parse error (malformed ports array)
	testContent := `
[server]
token = "secret123"
log_level = "info"
nodelay = true
keepalive_period = 30
ports = [
	"5000",
	"127.0.0.1:5001",
	"127.0.0.1:5002=test.example.com:5002"
	"this is malformed - missing comma"
]

[client]
token = "client_token_secret"
`

	// Create temporary file
	tmpfile, err := ioutil.TempFile("", "test_config_*.toml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.WriteString(testContent); err != nil {
		t.Fatal(err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatal(err)
	}

	// Test extraction
	serverToken := extractTokenFromFile(tmpfile.Name(), "server")
	if serverToken != "secret123" {
		t.Errorf("Expected server token 'secret123', got '%s'", serverToken)
	}

	clientToken := extractTokenFromFile(tmpfile.Name(), "client")
	if clientToken != "client_token_secret" {
		t.Errorf("Expected client token 'client_token_secret', got '%s'", clientToken)
	}
}

func TestDirectFileProviderWithMalformedConfig(t *testing.T) {
	// Create a test config file with a parse error
	testContent := `
[server]
token = "test_server_token"
log_level = "debug"

[client]
token = "test_client_token"
`

	// Create temporary file
	tmpfile, err := ioutil.TempFile("", "test_provider_*.toml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.WriteString(testContent); err != nil {
		t.Fatal(err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatal(err)
	}

	// Create provider
	provider := NewDirectFileConfigProvider(tmpfile.Name())

	// Check that tokens are cached even if TOML parsing fails
	serverCfg := provider.GetServerConfig()
	if serverCfg.Token != "test_server_token" {
		t.Errorf("Expected server token 'test_server_token', got '%s'", serverCfg.Token)
	}

	clientCfg := provider.GetClientConfig()
	if clientCfg.Token != "test_client_token" {
		t.Errorf("Expected client token 'test_client_token', got '%s'", clientCfg.Token)
	}
}
