package dotfiles

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type notificationHistoryEntry struct {
	ID        string `json:"id"`
	App       string `json:"app,omitempty"`
	Summary   string `json:"summary,omitempty"`
	Body      string `json:"body,omitempty"`
	Urgency   string `json:"urgency,omitempty"`
	Timestamp string `json:"timestamp"`
	Visible   bool   `json:"visible"`
	Dismissed bool   `json:"dismissed"`
	Source    string `json:"source,omitempty"`
}

func readNotificationHistoryEntries() ([]notificationHistoryEntry, error) {
	path := dotfilesNotificationHistoryLogPath()
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open notification history: %w", err)
	}
	defer file.Close()

	var entries []notificationHistoryEntry
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var entry notificationHistoryEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}
		if entry.ID == "" {
			entry.ID = fmt.Sprintf("legacy-%d", len(entries)+1)
		}
		if entry.Timestamp == "" {
			entry.Timestamp = time.Now().Format(time.RFC3339)
		}
		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan notification history: %w", err)
	}
	return entries, nil
}

func writeNotificationHistoryEntries(entries []notificationHistoryEntry) error {
	dir := dotfilesNotificationHistoryDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create notification history dir: %w", err)
	}

	path := dotfilesNotificationHistoryLogPath()
	tmpPath := filepath.Join(dir, "history.jsonl.tmp")
	file, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("create temp notification history: %w", err)
	}

	enc := json.NewEncoder(file)
	for _, entry := range entries {
		if entry.ID == "" {
			entry.ID = fmt.Sprintf("entry-%d", time.Now().UnixNano())
		}
		if entry.Timestamp == "" {
			entry.Timestamp = time.Now().Format(time.RFC3339)
		}
		if err := enc.Encode(entry); err != nil {
			file.Close()
			return fmt.Errorf("encode notification history: %w", err)
		}
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close temp notification history: %w", err)
	}
	return os.Rename(tmpPath, path)
}

func appendNotificationHistoryEntry(entry notificationHistoryEntry) error {
	if entry.ID == "" {
		entry.ID = fmt.Sprintf("entry-%d", time.Now().UnixNano())
	}
	if entry.Timestamp == "" {
		entry.Timestamp = time.Now().Format(time.RFC3339)
	}
	entry.Visible = true
	entry.Dismissed = false

	entries, err := readNotificationHistoryEntries()
	if err != nil {
		return err
	}
	entries = append(entries, entry)
	return writeNotificationHistoryEntries(entries)
}

func markNotificationHistoryDismissed(limit int) (int, error) {
	entries, err := readNotificationHistoryEntries()
	if err != nil {
		return 0, err
	}
	if len(entries) == 0 {
		return 0, nil
	}

	updated := 0
	for i := len(entries) - 1; i >= 0; i-- {
		if entries[i].Dismissed || !entries[i].Visible {
			continue
		}
		entries[i].Dismissed = true
		entries[i].Visible = false
		updated++
		if limit > 0 && updated >= limit {
			break
		}
	}
	if updated == 0 {
		return 0, nil
	}
	return updated, writeNotificationHistoryEntries(entries)
}

func clearNotificationHistory(purge bool) (int, error) {
	entries, err := readNotificationHistoryEntries()
	if err != nil {
		return 0, err
	}
	if len(entries) == 0 {
		return 0, nil
	}
	if purge {
		return len(entries), writeNotificationHistoryEntries(nil)
	}
	updated := 0
	for i := range entries {
		if entries[i].Dismissed && !entries[i].Visible {
			continue
		}
		entries[i].Dismissed = true
		entries[i].Visible = false
		updated++
	}
	if updated == 0 {
		return 0, nil
	}
	return updated, writeNotificationHistoryEntries(entries)
}
