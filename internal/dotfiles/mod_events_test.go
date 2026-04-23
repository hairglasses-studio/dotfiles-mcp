package dotfiles

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeEventsLog creates a fake events.jsonl with N synthetic entries
// spanning a small time range. Returns the temp path.
func writeEventsLog(t *testing.T, n int) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	now := time.Now().UTC()
	var b strings.Builder
	for i := 0; i < n; i++ {
		// Stagger timestamps 1 second apart so time-cutoff tests can
		// target a subset.
		at := now.Add(-time.Duration(n-i) * time.Second).Format(time.RFC3339)
		sev := "low"
		if i%10 == 0 {
			sev = "high"
		}
		fmt.Fprintf(&b, `{"type":"t%d","at":"%s","error_code":"code_%d","severity":"%s","rule":"r","source":"s"}`, i, at, i%3, sev)
		b.WriteByte('\n')
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatalf("write events log: %v", err)
	}
	return path
}

func TestEventsScanSmallLog(t *testing.T) {
	// Small log — both tail-scan and full-scan see the same data.
	path := writeEventsLog(t, 20)
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()
	records, err := eventsScan(f, 50, time.Time{}, 0, "")
	if err != nil {
		t.Fatalf("eventsScan: %v", err)
	}
	if len(records) != 20 {
		t.Errorf("want 20 records, got %d", len(records))
	}
}

func TestEventsScanLimitRespected(t *testing.T) {
	path := writeEventsLog(t, 200)
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()
	records, err := eventsScan(f, 10, time.Time{}, 0, "")
	if err != nil {
		t.Fatalf("eventsScan: %v", err)
	}
	if len(records) != 10 {
		t.Errorf("want 10 (limit), got %d", len(records))
	}
	// Tail slice — should be the last 10 by write order.
	if records[len(records)-1].Type != "t199" {
		t.Errorf("expected last record to be t199, got %q", records[len(records)-1].Type)
	}
}

func TestEventsScanSeverityFilter(t *testing.T) {
	// 200 records; every 10th is high. Expect 20 high results.
	path := writeEventsLog(t, 200)
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()
	records, err := eventsScan(f, 50, time.Time{}, severityRank("high"), "")
	if err != nil {
		t.Fatalf("eventsScan: %v", err)
	}
	if len(records) != 20 {
		t.Errorf("want 20 high-severity records, got %d", len(records))
	}
	for _, r := range records {
		if r.Severity != "high" {
			t.Errorf("severity filter leaked: %q", r.Severity)
		}
	}
}

func TestEventsScanTailFallback(t *testing.T) {
	// A narrow filter across a large log — the tail window may not find
	// enough matches; the fallback path must still return them.
	path := writeEventsLog(t, 5000)
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()
	// type "t7" exists once in 5000 records at position 7 — well outside
	// the 256 KB tail window for a log with ~250-byte entries.
	records, err := eventsScan(f, 5, time.Time{}, 0, "t7")
	if err != nil {
		t.Fatalf("eventsScan: %v", err)
	}
	if len(records) != 1 {
		t.Errorf("want 1 match for t7, got %d (fallback path may be broken)", len(records))
	}
	if len(records) == 1 && records[0].Type != "t7" {
		t.Errorf("filter leaked: got %q", records[0].Type)
	}
}

func TestEventsScanTimeCutoff(t *testing.T) {
	path := writeEventsLog(t, 100)
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()
	// Cutoff drops events older than 50 seconds — log entries are 1s
	// apart, so we expect roughly the last 50 entries.
	cutoff := time.Now().UTC().Add(-50 * time.Second)
	records, err := eventsScan(f, 200, cutoff, 0, "")
	if err != nil {
		t.Fatalf("eventsScan: %v", err)
	}
	if len(records) < 40 || len(records) > 60 {
		t.Errorf("want ~50 records within cutoff window, got %d", len(records))
	}
}
