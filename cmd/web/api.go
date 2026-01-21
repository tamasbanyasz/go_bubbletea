package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type BatchMeta struct {
	BatchID   string      `json:"batch_id"`
	CreatedAt string      `json:"created_at"`
	Count     int         `json:"count"`
	Workers   int         `json:"workers"`
	CSVFiles  CSVFiles    `json:"csv_files,omitempty"`
	Durations DurationsMS `json:"durations_ms,omitempty"`
	Queue     QueueMeta   `json:"queue,omitempty"`
	Memory    MemoryMeta  `json:"memory_bytes,omitempty"`
}

type CSVFiles struct {
	Name string `json:"name"`
	Num  string `json:"num"`
	Time string `json:"time"`
}

type DurationsMS struct {
	Generate int64 `json:"generate"`
	CSV      int64 `json:"csv"`
	Queue    int64 `json:"queue"`
}

type QueueMeta struct {
	EvenCount int    `json:"even_count"`
	Status    string `json:"status"`
	Error     string `json:"error,omitempty"`
}

type MemoryMeta struct {
	Alloc      uint64 `json:"alloc"`
	TotalAlloc uint64 `json:"total_alloc"`
	Sys        uint64 `json:"sys"`
}

type batchSummary struct {
	BatchID   string
	CreatedAt string
	Status    string
	EvenCount int
	ModTime   time.Time
	Error     string
}

type apiBatchSummary struct {
	BatchID   string     `json:"batch_id"`
	CreatedAt string     `json:"created_at"`
	Count     int        `json:"count"`
	Status    string     `json:"status"`
	Queue     QueueMeta  `json:"queue"`
	CSVFiles  CSVFiles   `json:"csv_files"`
	Memory    MemoryMeta `json:"memory_bytes"`
	UpdatedAt string     `json:"updated_at"`
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "ok",
		"time":   time.Now().Format(time.RFC3339),
	})
}

func handleBatches(dir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start, err := parseTimeParam(r.URL.Query().Get("start"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		end, err := parseTimeParam(r.URL.Query().Get("end"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		items, details, err := loadBatches(dir)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		filtered := make([]batchSummary, 0, len(items))
		for _, item := range items {
			if start == nil && end == nil {
				filtered = append(filtered, item)
				continue
			}
			createdAt, parseErr := time.Parse(time.RFC3339Nano, item.CreatedAt)
			if parseErr != nil {
				continue
			}
			if start != nil && createdAt.Before(*start) {
				continue
			}
			if end != nil && createdAt.After(*end) {
				continue
			}
			filtered = append(filtered, item)
		}

		limit := len(filtered)
		if raw := r.URL.Query().Get("limit"); raw != "" {
			if parsed, convErr := strconv.Atoi(raw); convErr == nil && parsed >= 0 {
				if parsed < limit {
					limit = parsed
				}
			}
		}

		resp := make([]apiBatchSummary, 0, limit)
		for i := 0; i < limit; i++ {
			item := filtered[i]
			meta, ok := details[item.BatchID]
			if !ok {
				continue
			}
			resp = append(resp, apiBatchSummary{
				BatchID:   item.BatchID,
				CreatedAt: item.CreatedAt,
				Count:     meta.Count,
				Status:    meta.Queue.Status,
				Queue:     meta.Queue,
				CSVFiles:  meta.CSVFiles,
				Memory:    meta.Memory,
				UpdatedAt: item.ModTime.Format(time.RFC3339),
			})
		}

		writeJSON(w, http.StatusOK, map[string]any{"items": resp})
	}
}

func handleBatchByID(dir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/api/batches/")
		if id == "" {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "batch_id required"})
			return
		}

		_, details, err := loadBatches(dir)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		meta, ok := details[id]
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}

		writeJSON(w, http.StatusOK, meta)
	}
}

func loadBatches(dir string) ([]batchSummary, map[string]BatchMeta, error) {
	pattern := filepath.Join(dir, "*.json")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return nil, nil, err
	}

	items := make([]batchSummary, 0, len(files))
	details := make(map[string]BatchMeta)

	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		wrapper := map[string]BatchMeta{}
		if err := json.Unmarshal(data, &wrapper); err != nil {
			continue
		}
		info, err := os.Stat(file)
		if err != nil {
			continue
		}
		for id, meta := range wrapper {
			if meta.BatchID == "" {
				meta.BatchID = id
			}
			status := meta.Queue.Status
			items = append(items, batchSummary{
				BatchID:   id,
				CreatedAt: meta.CreatedAt,
				Status:    status,
				EvenCount: meta.Queue.EvenCount,
				ModTime:   info.ModTime(),
				Error:     meta.Queue.Error,
			})
			details[id] = meta
			break
		}
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].ModTime.After(items[j].ModTime)
	})
	return items, details, nil
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(payload)
}

func parseTimeParam(raw string) (*time.Time, error) {
	if raw == "" {
		return nil, nil
	}
	if parsed, err := time.ParseInLocation("20060102-150405", raw, time.Local); err == nil {
		return &parsed, nil
	}
	if parsed, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return &parsed, nil
	}
	if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
		return &parsed, nil
	}
	return nil, fmt.Errorf("invalid time format: %s (use 20060102-150405 or RFC3339)", raw)
}
