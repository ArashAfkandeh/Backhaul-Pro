package utils

import (
    "net"
    "sync"
    "time"
)

// hybridState keeps per-destination information that allows us to make a
// hybrid selection: when we first see a source IP we route it to the backend
// with the fewest prior assignments, and thereafter we stick that source to
// the same backend. The counters are *not* decremented when connections
// close, so they represent historical load rather than current active
// connections; this keeps the logic simple and lock‑free for transports that
// don't report teardown events.

type srcEntry struct {
    idx      int
    lastSeen int64 // unix nano
}

type hybridState struct {
    mu       sync.Mutex
    counters []uint64          // one entry per choice
    srcMap   map[string]srcEntry // map[srcIP] -> entry
}

var lbStates sync.Map // map[string]*hybridState

// SelectBySrcDstHash returns one of the provided remote addresses. If more
// than one target is configured for a given local port we first check whether
// we've previously seen the source IP; if so the same index is returned (sticky
// behaviour). Otherwise, we pick the backend with the lowest counter value and
// record the decision. This gives a mostly balanced distribution while
// preserving affinity for repeated connections from the same client.
//
// The srcAddr parameter is used only to extract the source IP; dstAddr is used
// as the key for storing state. An empty choices slice yields an empty string.
// ResetState removes stored loadbalancer information for a specific
// destination address. This is useful when the set of clients or backends has
// changed and you want to forget previous assignments for that listener.
func ResetState(dstAddr string) {
    lbStates.Delete(dstAddr)
}

// ResetAllStates completely clears all stored state across all destinations.
// Calling this will make the next call to SelectBySrcDstHash behave as if it
// were starting from scratch (no history of prior srcIPs).
func ResetAllStates() {
    lbStates = sync.Map{}
}

// StartAutoCleanup launches a background goroutine that periodically scans the
// stored state and removes any srcIP entries that have not been seen for at
// least maxAge. The caller should call StopAutoCleanup on the returned channel
// when the process is shutting down to avoid goroutine leaks.
//
// The cleanup operates per-destination; if all src entries for a specific
// dstAddr are removed nothing else happens (counters remain).
func StartAutoCleanup(interval, maxAge time.Duration) (stop chan struct{}) {
    stop = make(chan struct{})
    go func() {
        ticker := time.NewTicker(interval)
        defer ticker.Stop()
        for {
            select {
            case <-ticker.C:
                now := time.Now().UnixNano()
                lbStates.Range(func(key, val interface{}) bool {
                    st := val.(*hybridState)
                    st.mu.Lock()
                    for src, entry := range st.srcMap {
                        if now-entry.lastSeen > maxAge.Nanoseconds() {
                            delete(st.srcMap, src)
                        }
                    }
                    st.mu.Unlock()
                    return true
                })
            case <-stop:
                return
            }
        }
    }()
    return stop
}

// StopAutoCleanup signals the goroutine returned by StartAutoCleanup to exit.
func StopAutoCleanup(stop chan struct{}) {
    close(stop)
}

func SelectBySrcDstHash(srcAddr, dstAddr string, choices []string) string {
    if len(choices) == 0 {
        return ""
    }
    if len(choices) == 1 {
        return choices[0]
    }

    // extract IP portion in case ports vary
    srcIP, _, _ := net.SplitHostPort(srcAddr)

    key := dstAddr
    v, _ := lbStates.LoadOrStore(key, &hybridState{
        counters: make([]uint64, len(choices)),
        srcMap:   make(map[string]srcEntry),
    })
    st := v.(*hybridState)

    st.mu.Lock()
    defer st.mu.Unlock()

    // reset state if the number of choices changes
    if len(st.counters) != len(choices) {
        st.counters = make([]uint64, len(choices))
        st.srcMap = make(map[string]srcEntry)
    }

    now := time.Now().UnixNano()
    if entry, ok := st.srcMap[srcIP]; ok {
        // existing client, update lastSeen and return same index
        entry.lastSeen = now
        st.srcMap[srcIP] = entry
        return choices[entry.idx]
    }

    // pick backend with smallest counter value
    minIdx := 0
    for i := 1; i < len(st.counters); i++ {
        if st.counters[i] < st.counters[minIdx] {
            minIdx = i
        }
    }
    st.counters[minIdx]++
    st.srcMap[srcIP] = srcEntry{idx: minIdx, lastSeen: now}
    return choices[minIdx]
}
