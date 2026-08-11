package history

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type QueryLog struct {
	ID        int       `json:"id"`
	Query     string    `json:"query"`
	Timestamp time.Time `json:"timestamp"`
	OpenedFile string   `json:"opened_file,omitempty"`
	OpenedLine int      `json:"opened_line,omitempty"`
}

type HistoryManager struct {
	filePath string
}

func NewHistoryManager(rootDir string) *HistoryManager {
	dir := filepath.Join(rootDir, ".semsearch")
	os.MkdirAll(dir, 0755)
	return &HistoryManager{
		filePath: filepath.Join(dir, "history.json"),
	}
}

func (h *HistoryManager) Load() ([]QueryLog, error) {
	data, err := os.ReadFile(h.filePath)
	if err != nil {
		return nil, nil
	}
	var logs []QueryLog
	if err := json.Unmarshal(data, &logs); err != nil {
		return nil, err
	}
	return logs, nil
}

func (h *HistoryManager) Add(query string) (int, error) {
	logs, _ := h.Load()
	newLog := QueryLog{
		ID:        len(logs) + 1,
		Query:     query,
		Timestamp: time.Now(),
	}
	logs = append(logs, newLog)

	data, err := json.MarshalIndent(logs, "", "  ")
	if err != nil {
		return 0, err
	}
	return newLog.ID, os.WriteFile(h.filePath, data, 0644)
}

func (h *HistoryManager) RecordAction(queryID int, file string, line int) error {
	logs, _ := h.Load()
	for i := range logs {
		if logs[i].ID == queryID {
			logs[i].OpenedFile = file
			logs[i].OpenedLine = line
			break
		}
	}
	data, err := json.MarshalIndent(logs, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(h.filePath, data, 0644)
}
