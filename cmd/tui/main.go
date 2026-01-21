package main

// Read README.md for more information

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/fsnotify/fsnotify"
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
	FilePath  string
	ModTime   time.Time
	Error     string
}

type model struct {
	dir               string
	items             []batchSummary
	details           map[string]BatchMeta
	selected          int
	view              string
	width             int
	height            int
	errText           string
	rebuildActive     bool
	rebuildErr        string
	watcher           *fsnotify.Watcher
	reloadPending     bool
	reloadTimerActive bool
	quitStyle         lipgloss.Style
	title             lipgloss.Style
	selectedS         lipgloss.Style
	successS          lipgloss.Style
	errorS            lipgloss.Style
}

type fileEventMsg struct {
	path string
}

type fileErrMsg struct {
	err error
}

type refreshMsg struct {
	items   []batchSummary
	details map[string]BatchMeta
	err     error
}

type rebuildQueueMsg struct {
	batchID string
	err     error
}

type watchReadyMsg struct {
	watcher *fsnotify.Watcher
}

type debounceMsg struct{}

// TUI main függvénye
func main() {
	m := model{
		dir:       ".",
		view:      "list",
		quitStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("241")),
		title:     lipgloss.NewStyle().Bold(true),
		selectedS: lipgloss.NewStyle().Background(lipgloss.Color("238")).Foreground(lipgloss.Color("15")).Bold(true),
		successS:  lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true),
		errorS:    lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true),
	}
	if _, err := tea.NewProgram(m, tea.WithAltScreen()).Run(); err != nil {
		fmt.Println("TUI error:", err)
		os.Exit(1)
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(initWatcherCmd(m.dir), loadCmd(m.dir))
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, tea.ClearScreen
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			if m.watcher != nil {
				_ = m.watcher.Close()
			}
			return m, tea.Quit
		case "enter":
			if len(m.items) > 0 {
				m.view = "detail"
			}
			return m, nil
		case "b", "esc":
			m.view = "list"
			return m, nil
		case "r":
			if m.view == "detail" && len(m.items) > 0 && m.selected < len(m.items) {
				item := m.items[m.selected]
				meta, ok := m.details[item.BatchID]
				if ok && isRebuildAllowed(meta) && !m.rebuildActive {
					m.rebuildActive = true
					m.rebuildErr = ""
					return m, rebuildQueueCmd(item.FilePath, meta)
				}
			}
			return m, nil
		case "up", "k":
			if m.selected > 0 {
				m.selected--
			}
			return m, nil
		case "down", "j":
			if m.selected < len(m.items)-1 {
				m.selected++
			}
			return m, nil
		}
	case watchReadyMsg:
		m.watcher = msg.watcher
		return m, watchCmd(m.watcher)
	case fileEventMsg:
		if strings.HasSuffix(strings.ToLower(msg.path), ".json") {
			m.reloadPending = true
			if !m.reloadTimerActive {
				m.reloadTimerActive = true
				return m, tea.Batch(debounceCmd(200*time.Millisecond), watchCmd(m.watcher))
			}
			return m, watchCmd(m.watcher)
		}
		return m, watchCmd(m.watcher)
	case fileErrMsg:
		m.errText = msg.err.Error()
		return m, watchCmd(m.watcher)
	case debounceMsg:
		m.reloadTimerActive = false
		if m.reloadPending {
			m.reloadPending = false
			return m, tea.Batch(loadCmd(m.dir), watchCmd(m.watcher))
		}
		return m, watchCmd(m.watcher)
	case refreshMsg:
		if msg.err != nil {
			m.errText = msg.err.Error()
			return m, nil
		}
		prevSelectedID := ""
		if m.selected >= 0 && m.selected < len(m.items) {
			prevSelectedID = m.items[m.selected].BatchID
		}
		m.items = msg.items
		m.details = msg.details
		if prevSelectedID != "" {
			found := -1
			for i, item := range m.items {
				if item.BatchID == prevSelectedID {
					found = i
					break
				}
			}
			if found >= 0 {
				m.selected = found
			}
		}
		if m.selected >= len(m.items) {
			m.selected = len(m.items) - 1
		}
		if m.selected < 0 {
			m.selected = 0
		}
		return m, nil
	case rebuildQueueMsg:
		if msg.err != nil {
			m.rebuildErr = msg.err.Error()
		} else {
			m.rebuildErr = ""
		}
		m.rebuildActive = false
		return m, nil
	}
	return m, nil
}

func (m model) View() string {
	if m.view == "detail" {
		return m.renderDetail()
	}
	return m.renderList()
}

func (m model) renderList() string {
	total := len(m.items)
	done := 0
	failed := 0
	for _, item := range m.items {
		if strings.ToLower(item.Status) == "sikeresen" {
			done++
		} else if item.Status != "" && strings.ToLower(item.Status) != "sikeresen" {
			failed++
		}
	}

	doneText := m.successS.Render(fmt.Sprintf("Kész: %d", done))
	failedText := m.errorS.Render(fmt.Sprintf("Hiba: %d", failed))
	doneShort := m.successS.Render(fmt.Sprintf("K:%d", done))
	failedShort := m.errorS.Render(fmt.Sprintf("H:%d", failed))
	headerText := fmt.Sprintf("Batches: %d | %s | %s", total, doneText, failedText)
	if m.width > 0 {
		switch {
		case m.width < 12:
			headerText = fmt.Sprintf("B:%d", total)
		case m.width < 20:
			headerText = fmt.Sprintf("B:%d %s", total, doneShort)
		case m.width < 30:
			headerText = fmt.Sprintf("B:%d %s %s", total, doneShort, failedShort)
		case m.width < 60:
			headerText = fmt.Sprintf("B:%d %s %s", total, doneShort, failedShort)
		}
	}
	headerLine := headerText
	if m.width > 0 {
		headerLine = ansi.Truncate(headerLine, m.width, "")
		if pad := m.width - lipgloss.Width(headerLine); pad > 0 {
			headerLine += strings.Repeat(" ", pad)
		}
	}
	header := m.title.Render(headerLine)
	help := m.quitStyle.Render("↑/↓ vagy j/k: navigáció • Enter: detail • b/esc: vissza • q: kilép")

	lines := []string{header, help, ""}
	if m.errText != "" {
		lines = append(lines, "Hiba: "+m.errText, "")
	}

	visible := 20
	if m.height > 6 {
		visible = m.height - 6
	}
	start := 0
	if m.selected > visible/2 {
		start = m.selected - visible/2
	}
	end := start + visible
	if end > len(m.items) {
		end = len(m.items)
	}
	if start > end {
		start = end
	}

	for i := start; i < end; i++ {
		item := m.items[i]
		prefix := "  "
		if i == m.selected {
			prefix = "> "
		}
		status := item.Status
		if status == "" {
			status = "Folyamatban!"
		}
		switch strings.ToLower(status) {
		case "sikeresen":
			status = m.successS.Render(status)
		case "hiba":
			status = m.errorS.Render(status)
		}
		line := fmt.Sprintf("%s%s | %s | even=%d | %s", prefix, item.BatchID, status, item.EvenCount, item.CreatedAt)
		if i == m.selected {
			line = m.selectedS.Render(line)
		}
		lines = append(lines, line)
	}

	if len(m.items) == 0 {
		lines = append(lines, "Nincs JSON batch fájl a mappában.")
	}

	return strings.Join(lines, "\n")
}

func (m model) renderDetail() string {
	if len(m.items) == 0 || m.selected >= len(m.items) {
		return "Nincs kiválasztott batch.\n\nNyomj b-t a visszalépéshez."
	}
	item := m.items[m.selected]
	meta, ok := m.details[item.BatchID]
	if !ok {
		return "A részletek nem érhetők el.\n\nNyomj b-t a visszalépéshez."
	}

	statusText := meta.Queue.Status
	switch strings.ToLower(statusText) {
	case "sikeresen":
		statusText = m.successS.Render(statusText)
	case "hiba":
		statusText = m.errorS.Render(statusText)
	}

	lines := []string{
		m.title.Render("Batch részletek"),
		"",
		fmt.Sprintf("BatchID:  %s", meta.BatchID),
		fmt.Sprintf("Created:  %s", meta.CreatedAt),
		fmt.Sprintf("Count:    %d", meta.Count),
		fmt.Sprintf("Workers:  %d", meta.Workers),
		"",
		fmt.Sprintf("Queue:    %s (even=%d)", statusText, meta.Queue.EvenCount),
	}
	if meta.Queue.Error != "" {
		lines = append(lines, fmt.Sprintf("Queue hiba: %s", meta.Queue.Error))
	}
	if m.rebuildActive && isRebuildAllowed(meta) {
		lines = append(lines, "Queue újrafuttatás folyamatban...")
	}
	if m.rebuildErr != "" {
		lines = append(lines, fmt.Sprintf("Újrafuttatás hiba: %s", m.rebuildErr))
	}
	lines = append(lines, "",
		fmt.Sprintf("CSV name: %s", meta.CSVFiles.Name),
		fmt.Sprintf("CSV num:  %s", meta.CSVFiles.Num),
		fmt.Sprintf("CSV time: %s", meta.CSVFiles.Time),
		"",
		fmt.Sprintf("Durations (ms): gen=%d csv=%d queue=%d", meta.Durations.Generate, meta.Durations.CSV, meta.Durations.Queue),
		fmt.Sprintf("Memory bytes: alloc=%d total=%d sys=%d", meta.Memory.Alloc, meta.Memory.TotalAlloc, meta.Memory.Sys),
		"",
		renderDetailHelp(meta),
	)
	return strings.Join(lines, "\n")
}

func renderDetailHelp(meta BatchMeta) string {
	if isRebuildAllowed(meta) {
		return "Nyomj r-t az újrafuttatáshoz, b-t a visszalépéshez."
	}
	return "Nyomj b-t a visszalépéshez."
}

func isRebuildAllowed(meta BatchMeta) bool {
	status := strings.ToLower(meta.Queue.Status)
	return status == "folyamatban" || status == "hiba"
}

type dataRecord struct {
	BatchID string
	Name    string
	Num     int
	At      time.Time
}

func rebuildQueueCmd(path string, meta BatchMeta) tea.Cmd {
	return func() tea.Msg {
		_ = updateBatchMetaQueueStatus(path, meta.BatchID, QueueMeta{Status: "folyamatban"})
		start := time.Now()
		queue, errCh := buildEvenQueue(meta.CSVFiles.Name, meta.CSVFiles.Num, meta.CSVFiles.Time, 4096)
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
		queueDuration := time.Since(start)
		var mem runtime.MemStats
		runtime.ReadMemStats(&mem)
		metaErr := updateBatchMetaQueueResult(
			path,
			meta.BatchID,
			QueueMeta{
				EvenCount: evenCount,
				Status:    queueStatus(queueErr),
				Error:     queueErrorText(queueErr),
			},
			queueDuration.Milliseconds(),
			MemoryMeta{
				Alloc:      mem.Alloc,
				TotalAlloc: mem.TotalAlloc,
				Sys:        mem.Sys,
			},
		)
		if metaErr != nil {
			return rebuildQueueMsg{batchID: meta.BatchID, err: metaErr}
		}
		if queueErr != nil {
			return rebuildQueueMsg{batchID: meta.BatchID, err: queueErr}
		}
		return rebuildQueueMsg{batchID: meta.BatchID, err: nil}
	}
}

func updateBatchMetaQueueStatus(path, batchID string, queue QueueMeta) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	wrapper := map[string]BatchMeta{}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return err
	}
	meta, ok := wrapper[batchID]
	if !ok {
		meta = BatchMeta{BatchID: batchID}
	}
	meta.Queue = queue
	wrapper[batchID] = meta
	payload, err := json.MarshalIndent(wrapper, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, payload, 0644)
}

func updateBatchMetaQueueResult(path, batchID string, queue QueueMeta, queueDuration int64, memory MemoryMeta) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	wrapper := map[string]BatchMeta{}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return err
	}
	meta, ok := wrapper[batchID]
	if !ok {
		meta = BatchMeta{BatchID: batchID}
	}
	meta.Queue = queue
	meta.Durations.Queue = queueDuration
	meta.Memory = memory
	wrapper[batchID] = meta
	payload, err := json.MarshalIndent(wrapper, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, payload, 0644)
}

func buildEvenQueue(namePath, numPath, timePath string, queueSize int) (<-chan dataRecord, <-chan error) {
	if queueSize <= 0 {
		queueSize = 1024
	}

	out := make(chan dataRecord, queueSize)
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

			out <- dataRecord{
				BatchID: batchID,
				Name:    nameRow[1],
				Num:     num,
				At:      at,
			}
		}
	}()

	return out, errCh
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

func initWatcherCmd(dir string) tea.Cmd {
	return func() tea.Msg {
		w, err := fsnotify.NewWatcher()
		if err != nil {
			return fileErrMsg{err: err}
		}
		if err := w.Add(dir); err != nil {
			return fileErrMsg{err: err}
		}
		return watchReadyMsg{watcher: w}
	}
}

func watchCmd(w *fsnotify.Watcher) tea.Cmd {
	return func() tea.Msg {
		if w == nil {
			time.Sleep(250 * time.Millisecond)
			return fileErrMsg{err: fmt.Errorf("watcher not ready")}
		}
		select {
		case ev := <-w.Events:
			return fileEventMsg{path: ev.Name}
		case err := <-w.Errors:
			return fileErrMsg{err: err}
		}
	}
}

func debounceCmd(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg {
		return debounceMsg{}
	})
}

func loadCmd(dir string) tea.Cmd {
	return func() tea.Msg {
		items, details, err := loadBatches(dir)
		return refreshMsg{items: items, details: details, err: err}
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
				FilePath:  file,
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
