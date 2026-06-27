package utils

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
)

// UpdateTOMLValue updates a single key-value pair in a TOML file while preserving structure
func UpdateTOMLValue(filePath string, section string, key string, value interface{}) error {
	// Read the entire file as a generic map
	var data map[string]interface{}
	if _, err := toml.DecodeFile(filePath, &data); err != nil {
		return err
	}

	// Ensure the section exists
	if _, exists := data[section]; !exists {
		data[section] = make(map[string]interface{})
	}

	// Get the section as a map
	sectionMap, ok := data[section].(map[string]interface{})
	if !ok {
		return fmt.Errorf("section '%s' is not a map", section)
	}

	// Update the value
	sectionMap[key] = value

	// Special handling for ports - write back to file with custom formatting
	if key == "ports" {
		return writeTOMLWithPortsAtEnd(filePath, data, section)
	}

	// Write back to file with custom formatting to keep ports at the end
	return writeTOMLWithPortsAtEnd(filePath, data, section)
}

// writeTOMLWithPortsAtEnd writes TOML data ensuring ports field is at the end with proper formatting
func writeTOMLWithPortsAtEnd(filePath string, data map[string]interface{}, section string) error {
	// Extract ports from data if it exists
	var portValues []string
	sectionMap := data[section].(map[string]interface{})
	if portsData, exists := sectionMap["ports"]; exists {
		// Convert ports to []string
		switch v := portsData.(type) {
		case []interface{}:
			for _, port := range v {
				if s, ok := port.(string); ok {
					portValues = append(portValues, s)
				}
			}
		case []string:
			portValues = v
		}
		// Remove ports from the data so encoder doesn't write it
		delete(sectionMap, "ports")
	}

	// First write normally (without ports)
	tmpFile := filePath + ".tmp"
	f, err := os.Create(tmpFile)
	if err != nil {
		return err
	}
	defer f.Close()

	encoder := toml.NewEncoder(f)
	if err := encoder.Encode(data); err != nil {
		f.Close()
		os.Remove(tmpFile)
		return err
	}
	f.Close()

	// Now read the file and add ports at the end with custom formatting
	file, err := os.Open(tmpFile)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var lines []string
	var inTargetSection bool
	var lastSectionLine int

	for scanner.Scan() {
		line := scanner.Text()
		lines = append(lines, line)

		// Check if we're in the target section
		if line == "["+section+"]" {
			inTargetSection = true
			lastSectionLine = len(lines) - 1
		} else if inTargetSection && strings.HasPrefix(line, "[") {
			inTargetSection = false
		}
	}

	if err := scanner.Err(); err != nil {
		os.Remove(tmpFile)
		return err
	}

	// Add ports at the end of target section if they exist
	if len(portValues) > 0 {
		// Find where to insert ports (before next section or at end)
		insertPos := len(lines)
		if !inTargetSection && lastSectionLine >= 0 {
			// Find the next section header
			for i := lastSectionLine + 1; i < len(lines); i++ {
				if strings.HasPrefix(lines[i], "[") {
					insertPos = i
					break
				}
			}
		}

		// Build ports output
		portsLines := []string{""}
		portsLines = append(portsLines, "ports = [")
		for i, port := range portValues {
			if i < len(portValues)-1 {
				portsLines = append(portsLines, fmt.Sprintf("  \"%s\",", port))
			} else {
				portsLines = append(portsLines, fmt.Sprintf("  \"%s\"", port))
			}
		}
		portsLines = append(portsLines, "]")

		// Insert ports
		newLines := append(lines[:insertPos], append(portsLines, lines[insertPos:]...)...)
		lines = newLines
	}

	// Write final file
	output, err := os.Create(filePath)
	if err != nil {
		os.Remove(tmpFile)
		return err
	}
	defer output.Close()

	writer := bufio.NewWriter(output)
	for _, line := range lines {
		writer.WriteString(line + "\n")
	}
	writer.Flush()

	// Clean up temp file
	os.Remove(tmpFile)

	return nil
}
