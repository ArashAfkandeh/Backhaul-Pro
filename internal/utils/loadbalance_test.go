package utils

import (
    "sync"
    "testing"
    "time"
)

func TestSelectBySrcDstHash_Single(t *testing.T) {
    choices := []string{"one"}
    got := SelectBySrcDstHash("1.2.3.4:1234", "5.6.7.8:80", choices)
    if got != "one" {
        t.Errorf("expected single choice to be returned, got %s", got)
    }
}

func TestSelectBySrcDstHash_Empty(t *testing.T) {
    got := SelectBySrcDstHash("a:b", "c:d", []string{})
    if got != "" {
        t.Errorf("expected empty string for empty choices, got %s", got)
    }
}

// clear internal state map between tests
func resetLB() {
    lbStates = sync.Map{}
}

func TestSelectBySrcDstHash_Sticky(t *testing.T) {
    choices := []string{"X", "Y", "Z"}
    dst := "10.0.0.2:2000"

    resetLB()
    src := "1.1.1.1:5000"
    first := SelectBySrcDstHash(src, dst, choices)
    for i := 0; i < 5; i++ {
        got := SelectBySrcDstHash(src, dst, choices)
        if got != first {
            t.Errorf("sticky behaviour broken: got %s, want %s", got, first)
        }
    }
}

func TestSelectBySrcDstHash_InitialLoad(t *testing.T) {
    choices := []string{"A", "B", "C"}
    dst := "10.0.0.2:2000"

    resetLB()
    srcs := []string{"1.1.1.1:1", "2.2.2.2:2", "3.3.3.3:3", "4.4.4.4:4"}
    gotIdx := make(map[int]int)
    for _, src := range srcs {
        sel := SelectBySrcDstHash(src, dst, choices)
        for idx, ch := range choices {
            if sel == ch {
                gotIdx[idx]++
            }
        }
    }
    // we expect the first three sources to occupy different backends and the
    // fourth to go back to index 0 (least used)
    if gotIdx[0] != 2 || gotIdx[1] != 1 || gotIdx[2] != 1 {
        t.Errorf("unexpected distribution: %v", gotIdx)
    }
}

func TestResetState(t *testing.T) {
    choices := []string{"A", "B"}
    dst := "10.0.0.2:2000"

    resetLB()
    src := "5.5.5.5:1234"
    _ = SelectBySrcDstHash(src, dst, choices)
    // resetting the state should not panic and will clear historical data.
    ResetState(dst)
    // after reset, calling again simply behaves like the very first call
    second := SelectBySrcDstHash(src, dst, choices)
    if second != "A" && second != "B" {
        t.Errorf("unexpected choice after reset: %s", second)
    }
}

func TestResetAllStates(t *testing.T) {
    choices := []string{"A", "B"}
    dst1 := "10.0.0.2:2000"
    dst2 := "10.0.0.3:3000"

    resetLB()
    src := "6.6.6.6:2222"
    _ = SelectBySrcDstHash(src, dst1, choices)
    _ = SelectBySrcDstHash(src, dst2, choices)
    ResetAllStates()
    // after clearing everything the new assignment must not panic and can be any
    _ = SelectBySrcDstHash(src, dst1, choices)
    _ = SelectBySrcDstHash(src, dst2, choices)
}

func TestAutoCleanup(t *testing.T) {
    choices := []string{"A", "B"}
    dst := "10.0.0.2:2000"

    resetLB()
    // insert an entry with old timestamp by directly manipulating state
    key := dst
    v, _ := lbStates.LoadOrStore(key, &hybridState{
        counters: make([]uint64, len(choices)),
        srcMap:   make(map[string]srcEntry),
    })
    st := v.(*hybridState)
    st.mu.Lock()
    st.srcMap["1.1.1.1"] = srcEntry{idx: 0, lastSeen: time.Now().Add(-10 * time.Minute).UnixNano()}
    st.srcMap["2.2.2.2"] = srcEntry{idx: 1, lastSeen: time.Now().UnixNano()}
    st.mu.Unlock()

    stop := StartAutoCleanup(10*time.Millisecond, 1*time.Second)
    time.Sleep(50 * time.Millisecond)
    StopAutoCleanup(stop)

    st.mu.Lock()
    defer st.mu.Unlock()
    if _, ok := st.srcMap["1.1.1.1"]; ok {
        t.Errorf("old entry was not cleaned")
    }
    if _, ok := st.srcMap["2.2.2.2"]; !ok {
        t.Errorf("recent entry was incorrectly removed")
    }
}
