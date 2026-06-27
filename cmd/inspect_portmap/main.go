package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/musix/backhaul/internal/utils"
)

func main() {
	ports := []string{
		"in1.infovip.top:6701=64.49.15.175:8080",
		"in2.infovip.top:6701=45.67.139.173:8080",
		"in3.infovip.top:6701=45.43.92.164:8080",
	}

	m := utils.ParsePortsToListenerConfig(ports)
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(m); err != nil {
		fmt.Println("encode error:", err)
	}
}
