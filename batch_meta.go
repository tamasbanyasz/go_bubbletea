package main

import (
	"encoding/json"
	"os"
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

func InitBatchMeta(path, batchID string, count, workers int, createdAt time.Time) error {
	meta := BatchMeta{
		BatchID:   batchID,
		CreatedAt: createdAt.Format(time.RFC3339Nano),
		Count:     count,
		Workers:   workers,
	}
	return writeBatchMeta(path, batchID, meta)
}

func UpdateBatchMeta(path, batchID string, csvFiles CSVFiles, durations DurationsMS, queue QueueMeta, memory MemoryMeta) error {
	meta := BatchMeta{
		BatchID: batchID,
	}

	data, err := os.ReadFile(path)
	if err == nil {
		var wrapper map[string]BatchMeta
		if jsonErr := json.Unmarshal(data, &wrapper); jsonErr == nil {
			if existing, ok := wrapper[batchID]; ok {
				meta = existing
			}
		}
	}

	meta.CSVFiles = csvFiles
	meta.Durations = durations
	meta.Queue = queue
	meta.Memory = memory

	return writeBatchMeta(path, batchID, meta)
}

func writeBatchMeta(path, batchID string, meta BatchMeta) error {
	wrapper := map[string]BatchMeta{
		batchID: meta,
	}
	payload, err := json.MarshalIndent(wrapper, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, payload, 0644)
}
