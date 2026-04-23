// mod_events.go — surface structured events from the dotfiles-event-bus
// daemon via MCP tools. The daemon writes to ~/.local/state/dotfiles/
// events.jsonl (see scripts/event-bus.py); this module reads that stream
// and returns filtered, bounded slices of it.
package dotfiles

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hairglasses-studio/mcpkit/handler"
	"github.com/hairglasses-studio/mcpkit/registry"
	"github.com/hairglasses-studio/mcpkit/resources"
	"github.com/mark3labs/mcp-go/mcp"
)

// tailSeekBudget bounds the byte window scanned from the end of the log
// for a typical events_tail call. 256 KB ≈ 1000 events at ~250 bytes
// each — well beyond the max limit=500. If the requested filter matches
// fewer events than `limit` within the window, we fall back to a full
// scan so small-N queries on a heavily filtered log still work.
const tailSeekBudget = 256 * 1024

// EventsTailInput selects which events to return.
type EventsTailInput struct {
	// SinceMinutes is the lookback window; 0 returns all events up to Limit.
	SinceMinutes int `json:"since_minutes,omitempty" jsonschema:"description=Return only events within this many minutes of now (0 = no time filter)"`
	// Type filters by the event type field (matches error_code in most rules).
	Type string `json:"type,omitempty" jsonschema:"description=Filter by event type (e.g. hypr_reload_induced_drm, audio_sink_lost). Omit for all types."`
	// Severity filters to low | medium | high when set.
	Severity string `json:"severity,omitempty" jsonschema:"enum=low,enum=medium,enum=high,description=Filter to events at or above this severity"`
	// Limit caps the number of results; default 50.
	Limit int `json:"limit,omitempty" jsonschema:"description=Max events to return (default: 50, max: 500)"`
}

// EventRecord mirrors the JSONL schema written by event-bus.py.
type EventRecord struct {
	Type        string          `json:"type"`
	At          string          `json:"at"`
	Fingerprint string          `json:"fingerprint,omitempty"`
	ErrorCode   string          `json:"error_code,omitempty"`
	Severity    string          `json:"severity,omitempty"`
	Rule        string          `json:"rule,omitempty"`
	Source      string          `json:"source,omitempty"`
	Correlation json.RawMessage `json:"correlation,omitempty"`
}

type EventsTailOutput struct {
	Events []EventRecord `json:"events"`
	Count  int           `json:"count"`
	Path   string        `json:"path"`
	Exists bool          `json:"exists"`
}

// EventsModule exposes the event-bus stream as a read-only MCP surface.
type EventsModule struct{}

func (m *EventsModule) Name() string { return "events" }
func (m *EventsModule) Description() string {
	return "Read structured events from the dotfiles-event-bus daemon. Each event carries a remediation error_code that consumers can dispatch via remediation_lookup."
}

func (m *EventsModule) Tools() []registry.ToolDefinition {
	return []registry.ToolDefinition{
		handler.TypedHandler[EventsTailInput, EventsTailOutput](
			"events_tail",
			"Tail structured events from ~/.local/state/dotfiles/events.jsonl. Filters by time window, type, and severity. Use this to drive /heal (pending fixes) and /canary (post-deploy liveness).",
			func(_ context.Context, input EventsTailInput) (EventsTailOutput, error) {
				path := eventsLogPath()
				out := EventsTailOutput{Path: path, Events: []EventRecord{}}

				f, err := os.Open(path)
				if err != nil {
					if os.IsNotExist(err) {
						// Not an error — the event bus may simply not have
						// started yet. Return an empty tail with exists=false.
						out.Exists = false
						return out, nil
					}
					return out, err
				}
				defer f.Close()
				out.Exists = true

				limit := input.Limit
				if limit <= 0 {
					limit = 50
				}
				if limit > 500 {
					limit = 500
				}

				var cutoff time.Time
				if input.SinceMinutes > 0 {
					cutoff = time.Now().Add(-time.Duration(input.SinceMinutes) * time.Minute)
				}

				minSeverity := severityRank(input.Severity)
				wantType := strings.TrimSpace(input.Type)

				all, err := eventsScan(f, limit, cutoff, minSeverity, wantType)
				if err != nil {
					return out, err
				}
				out.Events = all
				out.Count = len(all)
				return out, nil
			},
		),
	}
}

// eventsScan reads up to `limit` matching records from the events log,
// seeking from the end to bound the work for typical queries and falling
// back to a full scan if the tail window yields fewer matches than the
// caller asked for. Filters: `wantType` matches the Type OR ErrorCode
// field; `minSeverity > 0` gates on severity; non-zero `cutoff` drops
// events older than the time.
func eventsScan(f *os.File, limit int, cutoff time.Time, minSeverity int, wantType string) ([]EventRecord, error) {
	// Try tail-seek first. If we find at least `limit` matches in the
	// window, that's the answer. Otherwise fall through to a full scan
	// so a very-narrow filter on a large log still works.
	if tail, ok, err := tailScan(f, limit, cutoff, minSeverity, wantType); err != nil {
		return nil, err
	} else if ok {
		return tail, nil
	}
	// Full scan from the start — the fallback path. Seek(0) is cheap
	// and the scanner walks the whole file. Large logs still hit this
	// when the filter is very narrow; the cost is linear and acceptable
	// since the tail window already missed.
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("seek events.jsonl: %w", err)
	}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	var all []EventRecord
	for sc.Scan() {
		if rec, keep := eventsFilter(sc.Bytes(), cutoff, minSeverity, wantType); keep {
			all = append(all, rec)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("scan events.jsonl: %w", err)
	}
	if len(all) > limit {
		all = all[len(all)-limit:]
	}
	return all, nil
}

// tailScan reads the last `tailSeekBudget` bytes, splits on newlines,
// skips the first (likely partial) line, and filters forward. Returns
// (matches, true, nil) when we found enough; (nil, false, nil) when we
// didn't (caller should fall back). Errors bubble up directly.
func tailScan(f *os.File, limit int, cutoff time.Time, minSeverity int, wantType string) ([]EventRecord, bool, error) {
	size, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		return nil, false, err
	}
	offset := size - int64(tailSeekBudget)
	if offset < 0 {
		offset = 0
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return nil, false, err
	}
	buf, err := io.ReadAll(f)
	if err != nil {
		return nil, false, err
	}
	// If we seeked mid-line, drop the partial prefix.
	if offset > 0 {
		if idx := bytes.IndexByte(buf, '\n'); idx >= 0 {
			buf = buf[idx+1:]
		}
	}
	var matches []EventRecord
	for _, line := range bytes.Split(buf, []byte{'\n'}) {
		if len(line) == 0 {
			continue
		}
		if rec, keep := eventsFilter(line, cutoff, minSeverity, wantType); keep {
			matches = append(matches, rec)
		}
	}
	// If the window didn't cover enough matches AND we might have missed
	// matches older than our seek point, fall back.
	if offset > 0 && len(matches) < limit {
		return nil, false, nil
	}
	if len(matches) > limit {
		matches = matches[len(matches)-limit:]
	}
	return matches, true, nil
}

// eventsFilter applies the same rules both scan paths use. Returns
// (record, true) when the line matches and should be kept.
func eventsFilter(line []byte, cutoff time.Time, minSeverity int, wantType string) (EventRecord, bool) {
	var rec EventRecord
	if err := json.Unmarshal(line, &rec); err != nil {
		return rec, false
	}
	if wantType != "" && rec.Type != wantType && rec.ErrorCode != wantType {
		return rec, false
	}
	if minSeverity > 0 && severityRank(rec.Severity) < minSeverity {
		return rec, false
	}
	if !cutoff.IsZero() {
		if at, err := time.Parse(time.RFC3339, rec.At); err == nil && at.Before(cutoff) {
			return rec, false
		}
	}
	return rec, true
}

// PendingHighSummary is the JSON body for the dotfiles://events/pending-high
// resource. Designed to answer "is there anything" with three small fields
// rather than the full event stream — cheaper than the events_tail tool
// when the caller just needs a yes/no + a hint of what's on fire.
type PendingHighSummary struct {
	Count         int      `json:"count"`
	LatestAt      string   `json:"latest_at,omitempty"`
	TopErrorCodes []string `json:"top_error_codes,omitempty"`
	WindowHours   int      `json:"window_hours"`
}

// Resources exposes pending-high-severity events as a read-through MCP
// resource. /heal and /canary both want a one-shot "is there anything?"
// check — reading a resource with a stable URI is cheaper than a full
// events_tail tool call and keeps the semantic separation between "give
// me the summary" and "let me filter the stream."
func (m *EventsModule) Resources() []resources.ResourceDefinition {
	return []resources.ResourceDefinition{
		{
			Resource: mcp.NewResource(
				"dotfiles://events/pending-high",
				"Pending High-Severity Events",
				mcp.WithResourceDescription("Count + latest timestamp + top error codes of high-severity events within the last 24h. Drives /heal and /canary Tier 6 checks."),
				mcp.WithMIMEType("application/json"),
			),
			Handler: func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
				summary, err := summarizePendingHigh(24)
				if err != nil {
					return nil, fmt.Errorf("summarize pending events: %w", err)
				}
				data, _ := json.MarshalIndent(summary, "", "  ")
				return []mcp.ResourceContents{
					mcp.TextResourceContents{
						URI:      "dotfiles://events/pending-high",
						MIMEType: "application/json",
						Text:     string(data),
					},
				}, nil
			},
			Category: "events",
			Tags:     []string{"events", "heal", "canary", "observability"},
		},
	}
}

// Templates has no templated resources for this module; satisfy the interface.
func (m *EventsModule) Templates() []resources.TemplateDefinition { return nil }

// summarizePendingHigh reads the log and returns a compact summary over
// the last windowHours. Empty log = zero count; missing log = zero count
// (the bus may not be running yet). Never errors on structural absence —
// only on actual read failures.
func summarizePendingHigh(windowHours int) (PendingHighSummary, error) {
	summary := PendingHighSummary{WindowHours: windowHours}
	path := eventsLogPath()
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return summary, nil
		}
		return summary, err
	}
	defer f.Close()
	cutoff := time.Now().Add(-time.Duration(windowHours) * time.Hour)
	records, err := eventsScan(f, 500, cutoff, severityRank("high"), "")
	if err != nil {
		return summary, err
	}
	summary.Count = len(records)
	if len(records) == 0 {
		return summary, nil
	}
	summary.LatestAt = records[len(records)-1].At
	// Top codes by frequency.
	codeFreq := map[string]int{}
	for _, r := range records {
		code := r.ErrorCode
		if code == "" {
			code = r.Type
		}
		codeFreq[code]++
	}
	type kv struct {
		code  string
		count int
	}
	flat := make([]kv, 0, len(codeFreq))
	for c, n := range codeFreq {
		flat = append(flat, kv{c, n})
	}
	sort.Slice(flat, func(i, j int) bool {
		if flat[i].count == flat[j].count {
			return flat[i].code < flat[j].code
		}
		return flat[i].count > flat[j].count
	})
	top := flat
	if len(top) > 5 {
		top = top[:5]
	}
	summary.TopErrorCodes = make([]string, len(top))
	for i, e := range top {
		summary.TopErrorCodes[i] = e.code
	}
	return summary, nil
}

func eventsLogPath() string {
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		base = filepath.Join(homeDir(), ".local", "state")
	}
	return filepath.Join(base, "dotfiles", "events.jsonl")
}

// severityRank returns an ordering so "low" < "medium" < "high". Unknown
// values rank 0 so they pass any min-severity filter.
func severityRank(s string) int {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "low":
		return 1
	case "medium":
		return 2
	case "high":
		return 3
	default:
		return 0
	}
}
