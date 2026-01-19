package main

// Read README.md for more information

import (
	"math/rand/v2"
	"runtime"
	"strconv"
	"sync"
	"time"
)

// DataRecord represents a single data item with name, number, and time.
type DataRecord struct {
	BatchID string
	Name    string
	Num     int
	At      time.Time
}

// NewDataRecord creates a DataRecord with the provided values.
func NewDataRecord(name string, num int, at time.Time) DataRecord {
	return DataRecord{
		Name: name,
		Num:  num,
		At:   at,
	}
}

// GenerateDataRecords creates the requested number of records concurrently.
// It returns a stream of records and closes it when done.
func GenerateDataRecords(count int, workers int) (string, <-chan DataRecord, int, error) {
	if count <= 0 {
		return "", nil, 0, nil
	}
	if workers <= 0 {
		workers = runtime.NumCPU()
	}
	if workers > count {
		workers = count
	}

	startTime := time.Now()
	batchID := startTime.Format("20060102-150405") + "-" + strconv.FormatInt(startTime.UnixNano(), 10)
	jsonPath := batchID + ".json"
	if err := InitBatchMeta(jsonPath, batchID, count, workers, startTime); err != nil {
		return batchID, nil, workers, err
	}

	records := make(chan DataRecord, workers*4)
	jobs := make(chan struct{}, workers*4)

	var wg sync.WaitGroup
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		go func(workerID int) {
			defer wg.Done()
			seed1 := uint64(time.Now().UnixNano()) + uint64(workerID)
			seed2 := uint64(time.Now().UnixNano()) ^ uint64(workerID<<1)
			rng := rand.New(rand.NewPCG(seed1, seed2))
			for range jobs {
				records <- DataRecord{
					BatchID: batchID,
					Name:    "Teszt",
					Num:     rng.IntN(1000),
					At:      time.Now(),
				}
			}
		}(w)
	}

	go func() {
		for i := 0; i < count; i++ {
			jobs <- struct{}{}
		}
		close(jobs)
	}()

	go func() {
		wg.Wait()
		close(records)
	}()

	return batchID, records, workers, nil
}
