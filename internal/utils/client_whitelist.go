package utils

import (
	"fmt"
	"net"
	"strings"
)

// IsClientAllowed checks if a client IP is in the whitelist.
// If allowedClients is empty, all clients are allowed.
// Supports:
// - IPv4 addresses (192.168.1.1)
// - IPv6 addresses (::1 or in bracket notation [::1])
// - CIDR ranges (192.168.1.0/24 or 2001:db8::/32)
func IsClientAllowed(clientAddr string, allowedClients []string) bool {
	// If no whitelist is configured, allow all clients
	if len(allowedClients) == 0 {
		return true
	}

	// Extract IP from address (remove port if present)
	clientIP := extractIP(clientAddr)
	if clientIP == "" {
		return false
	}

	// Parse the client IP
	parsedIP := net.ParseIP(clientIP)
	if parsedIP == nil {
		return false
	}

	for _, allowed := range allowedClients {
		allowed = strings.TrimSpace(allowed)
		if allowed == "" {
			continue
		}

		// Check if it's a CIDR range
		if strings.Contains(allowed, "/") {
			_, cidr, err := net.ParseCIDR(allowed)
			if err == nil && cidr.Contains(parsedIP) {
				return true
			}
			continue
		}

		// Check exact IP match
		allowedIP := net.ParseIP(allowed)
		if allowedIP != nil && parsedIP.Equal(allowedIP) {
			return true
		}
	}

	return false
}

// extractIP extracts the IP address from a client address string.
// Handles formats like:
// - IPv4: "192.168.1.1" or "192.168.1.1:8080"
// - IPv6: "::1" or "[::1]:8080" or "2001:db8::1" or "[2001:db8::1]:8080"
func extractIP(clientAddr string) string {
	// Remove brackets for IPv6 addresses
	if strings.HasPrefix(clientAddr, "[") {
		endIdx := strings.Index(clientAddr, "]")
		if endIdx != -1 {
			return clientAddr[1:endIdx]
		}
	}

	// For IPv4 or unbracketed IPv6, find the last colon
	lastColon := strings.LastIndex(clientAddr, ":")
	if lastColon == -1 {
		// No port, return as-is
		return clientAddr
	}

	// Check if the part before last colon is IPv6 (contains colons)
	beforeColon := clientAddr[:lastColon]
	if strings.Count(beforeColon, ":") > 0 {
		// This is IPv6 without port
		return beforeColon
	}

	// This is IPv4 with port
	return beforeColon
}

// ValidateAllowedClients validates the whitelist format
// Returns error if any entry is invalid CIDR or IP
func ValidateAllowedClients(allowedClients []string) error {
	for _, client := range allowedClients {
		client = strings.TrimSpace(client)
		if client == "" {
			continue
		}

		// Try parsing as CIDR
		if strings.Contains(client, "/") {
			_, _, err := net.ParseCIDR(client)
			if err != nil {
				return fmt.Errorf("invalid CIDR range '%s': %w", client, err)
			}
			continue
		}

		// Try parsing as IP
		if net.ParseIP(client) == nil {
			return fmt.Errorf("invalid IP address '%s'", client)
		}
	}
	return nil
}
