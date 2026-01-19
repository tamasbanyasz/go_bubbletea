package main

// Read README.md for more information

import (
	"bufio"
	"encoding/csv"
	"os"
	"strconv"
	"sync"
	"time"
)

// WriteRecordsSplitCSV writes name, num, and time into separate CSV files.
// Each row shares the same batch_id so they can be joined later.
func WriteRecordsSplitCSV(records []DataRecord, namePath, numPath, timePath string) error {
	nameFile, err := os.Create(namePath)
	if err != nil {
		return err
	}
	defer nameFile.Close()

	numFile, err := os.Create(numPath)
	if err != nil {
		return err
	}
	defer numFile.Close()

	timeFile, err := os.Create(timePath)
	if err != nil {
		return err
	}
	defer timeFile.Close()

	nameBuf := bufio.NewWriterSize(nameFile, 1024*1024)
	numBuf := bufio.NewWriterSize(numFile, 1024*1024)
	timeBuf := bufio.NewWriterSize(timeFile, 1024*1024)
	defer nameBuf.Flush()
	defer numBuf.Flush()
	defer timeBuf.Flush()

	nameWriter := csv.NewWriter(nameBuf)
	numWriter := csv.NewWriter(numBuf)
	timeWriter := csv.NewWriter(timeBuf)

	if err := nameWriter.Write([]string{"batch_id", "name"}); err != nil {
		return err
	}
	if err := numWriter.Write([]string{"batch_id", "num"}); err != nil {
		return err
	}
	if err := timeWriter.Write([]string{"batch_id", "time"}); err != nil {
		return err
	}

	errCh := make(chan error, 3)
	var wg sync.WaitGroup
	wg.Add(3)

	go func() {
		defer wg.Done()
		for _, record := range records {
			if err := nameWriter.Write([]string{record.BatchID, record.Name}); err != nil {
				errCh <- err
				return
			}
		}
		nameWriter.Flush()
		if err := nameWriter.Error(); err != nil {
			errCh <- err
		}
	}()

	go func() {
		defer wg.Done()
		for _, record := range records {
			if err := numWriter.Write([]string{record.BatchID, strconv.Itoa(record.Num)}); err != nil {
				errCh <- err
				return
			}
		}
		numWriter.Flush()
		if err := numWriter.Error(); err != nil {
			errCh <- err
		}
	}()

	go func() {
		defer wg.Done()
		for _, record := range records {
			if err := timeWriter.Write([]string{record.BatchID, record.At.Format(time.RFC3339)}); err != nil {
				errCh <- err
				return
			}
		}
		timeWriter.Flush()
		if err := timeWriter.Error(); err != nil {
			errCh <- err
		}
	}()

	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			return err
		}
	}
	if err := nameBuf.Flush(); err != nil {
		return err
	}
	if err := numBuf.Flush(); err != nil {
		return err
	}
	if err := timeBuf.Flush(); err != nil {
		return err
	}
	return nil
}

// WriteRecordsSplitCSVStream writes records from a stream to CSV files without
// holding all records in memory. It returns a sample record and the count.
func WriteRecordsSplitCSVStream(records <-chan DataRecord, namePath, numPath, timePath string) (DataRecord, int, error) {
	nameFile, err := os.Create(namePath)
	if err != nil {
		return DataRecord{}, 0, err
	}
	defer nameFile.Close()

	numFile, err := os.Create(numPath)
	if err != nil {
		return DataRecord{}, 0, err
	}
	defer numFile.Close()

	timeFile, err := os.Create(timePath)
	if err != nil {
		return DataRecord{}, 0, err
	}
	defer timeFile.Close()

	nameBuf := bufio.NewWriterSize(nameFile, 1024*1024)
	numBuf := bufio.NewWriterSize(numFile, 1024*1024)
	timeBuf := bufio.NewWriterSize(timeFile, 1024*1024)
	defer nameBuf.Flush()
	defer numBuf.Flush()
	defer timeBuf.Flush()

	nameWriter := csv.NewWriter(nameBuf)
	numWriter := csv.NewWriter(numBuf)
	timeWriter := csv.NewWriter(timeBuf)

	if err := nameWriter.Write([]string{"batch_id", "name"}); err != nil {
		return DataRecord{}, 0, err
	}
	if err := numWriter.Write([]string{"batch_id", "num"}); err != nil {
		return DataRecord{}, 0, err
	}
	if err := timeWriter.Write([]string{"batch_id", "time"}); err != nil {
		return DataRecord{}, 0, err
	}

	count := 0
	sample := DataRecord{}
	for record := range records {
		if count == 0 {
			sample = record
		}
		count++

		if err := nameWriter.Write([]string{record.BatchID, record.Name}); err != nil {
			return sample, count, err
		}
		if err := numWriter.Write([]string{record.BatchID, strconv.Itoa(record.Num)}); err != nil {
			return sample, count, err
		}
		if err := timeWriter.Write([]string{record.BatchID, record.At.Format(time.RFC3339)}); err != nil {
			return sample, count, err
		}
	}

	nameWriter.Flush()
	if err := nameWriter.Error(); err != nil {
		return sample, count, err
	}
	numWriter.Flush()
	if err := numWriter.Error(); err != nil {
		return sample, count, err
	}
	timeWriter.Flush()
	if err := timeWriter.Error(); err != nil {
		return sample, count, err
	}
	if err := nameBuf.Flush(); err != nil {
		return sample, count, err
	}
	if err := numBuf.Flush(); err != nil {
		return sample, count, err
	}
	if err := timeBuf.Flush(); err != nil {
		return sample, count, err
	}

	return sample, count, nil
}
