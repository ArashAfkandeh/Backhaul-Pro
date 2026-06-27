package utils

import (
    "fmt"
    "net"
    "strconv"
    "strings"
)

// ListenerConfig holds the routing configuration for a single listener
// address (typically a port like ":1001").
type ListenerConfig struct {
    Remotes []string          // fallback / load-balance targets
    HostMap map[string]string // host -> target when hostname-based mapping used
}

// ParsePortsToListenerConfig parses the legacy `ports` array and produces a
// mapping of listener address -> ListenerConfig. Hostnames on the left-hand
// side (e.g. "input.example:1001=dest:2001") are recorded in HostMap while
// the listener is bound to the wildcard port ":1001" so a single listener
// can demultiplex by Host/SNI.
func ParsePortsToListenerConfig(ports []string) map[string]ListenerConfig {
    mapping := make(map[string]ListenerConfig)

    addEntry := func(laddr, raddr string) {
        cfg := mapping[laddr]
        cfg.Remotes = append(cfg.Remotes, raddr)
        mapping[laddr] = cfg
    }
    addHost := func(laddr, host, raddr string) {
        cfg := mapping[laddr]
        if cfg.HostMap == nil {
            cfg.HostMap = make(map[string]string)
        }
        cfg.HostMap[strings.ToLower(host)] = raddr
        mapping[laddr] = cfg
    }

    for _, portMapping := range ports {
        parts := strings.SplitN(portMapping, "=", 2)
        localPart := strings.TrimSpace(parts[0])
        var remoteAddr string
        if len(parts) == 2 {
            remoteAddr = strings.TrimSpace(parts[1])
        } else {
            remoteAddr = localPart
        }

        // numeric range (e.g. 8000-8010)
        if strings.Contains(localPart, "-") && !strings.Contains(localPart, ":") {
            rangeParts := strings.Split(localPart, "-")
            if len(rangeParts) != 2 {
                continue
            }
            startPort, err1 := strconv.Atoi(strings.TrimSpace(rangeParts[0]))
            endPort, err2 := strconv.Atoi(strings.TrimSpace(rangeParts[1]))
            if err1 != nil || err2 != nil || startPort < 1 || endPort < startPort {
                continue
            }
            for port := startPort; port <= endPort; port++ {
                laddr := fmt.Sprintf(":%d", port)
                addEntry(laddr, strconv.Itoa(port))
            }
            continue
        }

        // single numeric port
        if p, err := strconv.Atoi(localPart); err == nil {
            laddr := fmt.Sprintf(":%d", p)
            addEntry(laddr, remoteAddr)
            continue
        }

        // try host:port
        host, portStr, err := net.SplitHostPort(localPart)
        if err != nil {
            // fall back to raw entry
            addEntry(localPart, remoteAddr)
            continue
        }

        // Always bind to wildcard on the port (0.0.0.0). If a domain name was
        // provided use HostMap to remember the mapping.
        laddr := fmt.Sprintf(":%s", portStr)
        addEntry(laddr, remoteAddr)
        if host != "" && net.ParseIP(host) == nil {
            addHost(laddr, host, remoteAddr)
        }
    }

    return mapping
}
