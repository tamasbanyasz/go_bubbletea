package main

// Read README.md for more information

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
	watcher           *fsnotify.Watcher
	reloadPending     bool
	reloadTimerActive bool
	quitStyle         lipgloss.Style
	title             lipgloss.Style
	selectedS         lipgloss.Style
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
		selectedS: lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true),
	}
	if _, err := tea.NewProgram(m).Run(); err != nil {
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
		return m, nil
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

	header := m.title.Render(fmt.Sprintf("Batches: %d | Kész: %d | Hiba: %d", total, done, failed))
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

	lines := []string{
		m.title.Render("Batch részletek"),
		"",
		fmt.Sprintf("BatchID:  %s", meta.BatchID),
		fmt.Sprintf("Created:  %s", meta.CreatedAt),
		fmt.Sprintf("Count:    %d", meta.Count),
		fmt.Sprintf("Workers:  %d", meta.Workers),
		"",
		fmt.Sprintf("Queue:    %s (even=%d)", meta.Queue.Status, meta.Queue.EvenCount),
	}
	if meta.Queue.Error != "" {
		lines = append(lines, fmt.Sprintf("Queue hiba: %s", meta.Queue.Error))
	}
	lines = append(lines, "",
		fmt.Sprintf("CSV name: %s", meta.CSVFiles.Name),
		fmt.Sprintf("CSV num:  %s", meta.CSVFiles.Num),
		fmt.Sprintf("CSV time: %s", meta.CSVFiles.Time),
		"",
		fmt.Sprintf("Durations (ms): gen=%d csv=%d queue=%d", meta.Durations.Generate, meta.Durations.CSV, meta.Durations.Queue),
		fmt.Sprintf("Memory bytes: alloc=%d total=%d sys=%d", meta.Memory.Alloc, meta.Memory.TotalAlloc, meta.Memory.Sys),
		"",
		"Nyomj b-t a visszalépéshez.",
	)
	return strings.Join(lines, "\n")
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
