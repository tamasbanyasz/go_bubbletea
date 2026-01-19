package main

// Read README.md for more information

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// Csak az adat-előállítás main függvénye
func main() {
	count := 1000000
	interval := 1 * time.Second

	for {
		if err := runBatch(count); err != nil {
			fmt.Printf("Batch error: %v\n", err)
		}
		time.Sleep(interval)
	}
}

func runBatch(count int) error {
	start := time.Now()
	batchID, records, workers, err := GenerateDataRecords(count, 0)
	if err != nil {
		return fmt.Errorf("init batch: %w", err)
	}

	datePrefix := batchID
	if len(batchID) >= 8 {
		datePrefix = batchID[:8]
	}
	batchDir := filepath.Join("batches", batchID)
	if err := os.MkdirAll(batchDir, 0755); err != nil {
		return fmt.Errorf("batch dir: %w", err)
	}
	namePath := filepath.Join(batchDir, fmt.Sprintf("%s_%s_records_name.csv", datePrefix, batchID))
	numPath := filepath.Join(batchDir, fmt.Sprintf("%s_%s_records_num.csv", datePrefix, batchID))
	timePath := filepath.Join(batchDir, fmt.Sprintf("%s_%s_records_time.csv", datePrefix, batchID))
	jsonPath := batchID + ".json"
	_ = UpdateBatchMeta(
		jsonPath,
		batchID,
		CSVFiles{Name: namePath, Num: numPath, Time: timePath},
		DurationsMS{},
		QueueMeta{Status: "folyamatban"},
		MemoryMeta{},
	)

	csvStart := time.Now()
	sample, processed, err := WriteRecordsSplitCSVStream(records, namePath, numPath, timePath)
	if err != nil {
		queueErr := fmt.Errorf("csv write: %w", err)
		genDuration := time.Since(start)
		var mem runtime.MemStats
		runtime.ReadMemStats(&mem)
		_ = UpdateBatchMeta(
			jsonPath,
			batchID,
			CSVFiles{Name: namePath, Num: numPath, Time: timePath},
			DurationsMS{Generate: genDuration.Milliseconds()},
			QueueMeta{
				EvenCount: 0,
				Status:    queueStatus(queueErr),
				Error:     queueErrorText(queueErr),
			},
			MemoryMeta{
				Alloc:      mem.Alloc,
				TotalAlloc: mem.TotalAlloc,
				Sys:        mem.Sys,
			},
		)
		return queueErr
	}
	csvDuration := time.Since(csvStart)
	genDuration := time.Since(start)

	queueStart := time.Now()
	queue, errCh := BuildEvenQueue(namePath, numPath, timePath, 4096)
	evenCount := 0
	for range queue {
		evenCount++
	}
	queueErr := error(nil)
	for err := range errCh {
		if err != nil {
			queueErr = err
		}
	}
	queueDuration := time.Since(queueStart)

	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	metaErr := UpdateBatchMeta(
		jsonPath,
		batchID,
		CSVFiles{Name: namePath, Num: numPath, Time: timePath},
		DurationsMS{
			Generate: genDuration.Milliseconds(),
			CSV:      csvDuration.Milliseconds(),
			Queue:    queueDuration.Milliseconds(),
		},
		QueueMeta{
			EvenCount: evenCount,
			Status:    queueStatus(queueErr),
			Error:     queueErrorText(queueErr),
		},
		MemoryMeta{
			Alloc:      mem.Alloc,
			TotalAlloc: mem.TotalAlloc,
			Sys:        mem.Sys,
		},
	)
	if metaErr != nil {
		return fmt.Errorf("batch meta: %w", metaErr)
	}
	if queueErr != nil {
		return fmt.Errorf("queue build: %w", queueErr)
	}

	printStart := time.Now()
	fmt.Printf("Generated %d records in %s\n", processed, genDuration)
	fmt.Printf("Workers used: %d\n", workers)
	fmt.Printf("CSV written in %s (batch_id=%s)\n", csvDuration, batchID)
	fmt.Printf("Even queue size: %d in %s\n", evenCount, queueDuration)
	if processed > 0 {
		fmt.Printf("Sample: name=%s num=%d time=%s\n",
			sample.Name, sample.Num, sample.At.Format(time.RFC3339))
	}
	printDuration := time.Since(printStart)
	fmt.Printf("Print time: %s\n", printDuration)
	return nil
}

func queueStatus(err error) string {
	if err != nil {
		return "hiba"
	}
	return "sikeresen"
}

func queueErrorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
