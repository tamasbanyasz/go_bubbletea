# Go batch + TUI

This project generates batches of `DataRecord` items, writes them into three
separate CSV files, builds a queue from even `num` values, and exposes status
in a JSON file. A Bubble Tea TUI watches the JSON files in real time.

## Requirements

- Go 1.22+

Dependencies (installed via `go mod tidy`):
- `github.com/charmbracelet/bubbletea` v1.3.10
- `github.com/charmbracelet/lipgloss` v1.1.0
- `github.com/fsnotify/fsnotify` v1.9.0

## Project structure

- `main.go` — batch generator (records → CSV → even-queue → JSON meta)
- `data_record.go` — record generation (concurrent, streaming)
- `csv_output.go` — CSV writers (buffered)
- `even_queue.go` — even-number queue builder (streamed)
- `batch_meta.go` — JSON metadata create/update
- `cmd/tui/main.go` — TUI monitor (live file watch + detail view)

## How it works

1. **Batch generation** (`GenerateDataRecords`)
   - Creates a `batchID` with timestamp.
   - Initializes `<batchID>.json`.
   - Streams records via a channel.
2. **CSV writing** (`WriteRecordsSplitCSVStream`)
   - Writes three CSVs: name, num, time.
   - Buffered IO for speed.
3. **Even queue** (`BuildEvenQueue`)
   - Reads CSVs in lockstep.
   - Enqueues records with even `num`.
4. **JSON update** (`UpdateBatchMeta`)
   - Writes CSV paths, timings, queue status, and memory stats.
5. **TUI**
   - Watches JSON files live and shows list + detail view.

## Output

Each batch is written to its own folder:

```
batches/<batchID>/
  YYYYMMDD_<batchID>_records_name.csv
  YYYYMMDD_<batchID>_records_num.csv
  YYYYMMDD_<batchID>_records_time.csv
<batchID>.json
```

CSV headers:
- `records_name.csv`: `batch_id,name`
- `records_num.csv`: `batch_id,num`
- `records_time.csv`: `batch_id,time`

## Run

Batch generator (continuous loop):

```
go run .
```

TUI (monitor):

```
go run ./cmd/tui
```

### TUI controls

- `↑/↓` or `j/k` — move
- `Enter` — detail view
- `b` or `Esc` — back
- `q` — quit

## Notes

- The generator loops with a 1s interval between batches.
- JSON status starts as `folyamatban`, then becomes `sikeresen` or `hiba`.
