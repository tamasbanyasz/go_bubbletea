package main

import (
	"bufio"
	"encoding/csv"
	"io"
	"os"
	"strconv"
	"time"
)

// BuildEvenQueue reads the three CSV files in lockstep and enqueues records
// where num is even. It uses batch_id to validate alignment across files.
func BuildEvenQueue(namePath, numPath, timePath string, queueSize int) (<-chan DataRecord, <-chan error) {
	if queueSize <= 0 {
		queueSize = 1024
	}

	out := make(chan DataRecord, queueSize)
	errCh := make(chan error, 1)

	go func() {
		defer close(out)
		defer close(errCh)

		nameFile, err := os.Open(namePath)
		if err != nil {
			errCh <- err
			return
		}
		defer nameFile.Close()

		numFile, err := os.Open(numPath)
		if err != nil {
			errCh <- err
			return
		}
		defer numFile.Close()

		timeFile, err := os.Open(timePath)
		if err != nil {
			errCh <- err
			return
		}
		defer timeFile.Close()

		nameBuf := bufio.NewReaderSize(nameFile, 1024*1024)
		numBuf := bufio.NewReaderSize(numFile, 1024*1024)
		timeBuf := bufio.NewReaderSize(timeFile, 1024*1024)

		nameReader := csv.NewReader(nameBuf)
		numReader := csv.NewReader(numBuf)
		timeReader := csv.NewReader(timeBuf)
	nameReader.FieldsPerRecord = -1
	numReader.FieldsPerRecord = -1
	timeReader.FieldsPerRecord = -1
	nameReader.LazyQuotes = true
	numReader.LazyQuotes = true
	timeReader.LazyQuotes = true

		if _, err := nameReader.Read(); err != nil {
			errCh <- err
			return
		}
		if _, err := numReader.Read(); err != nil {
			errCh <- err
			return
		}
		if _, err := timeReader.Read(); err != nil {
			errCh <- err
			return
		}

		for {
			numRow, err := numReader.Read()
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				break
			}
			if err != nil {
				errCh <- err
				return
			}
			nameRow, err := nameReader.Read()
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				break
			}
			if err != nil {
				errCh <- err
				return
			}
			timeRow, err := timeReader.Read()
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				break
			}
			if err != nil {
				errCh <- err
				return
			}

			if len(numRow) < 2 || len(nameRow) < 2 || len(timeRow) < 2 {
				continue
			}

			batchID := numRow[0]
			if nameRow[0] != batchID || timeRow[0] != batchID {
				errCh <- io.ErrUnexpectedEOF
				return
			}

			num, err := strconv.Atoi(numRow[1])
			if err != nil {
				errCh <- err
				return
			}

			if num%2 != 0 {
				continue
			}

			at, err := time.Parse(time.RFC3339, timeRow[1])
			if err != nil {
				errCh <- err
				return
			}

			out <- DataRecord{
				BatchID: batchID,
				Name:    nameRow[1],
				Num:     num,
				At:      at,
			}
		}
	}()

	return out, errCh
}
