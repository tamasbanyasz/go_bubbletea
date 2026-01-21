package main

import (
	"bufio"
	"bytes"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type responseMsg struct {
	status   string
	body     string
	err      error
	duration time.Duration
}

type previewAnimMsg struct{}

type model struct {
	methods            []string
	methodIdx          int
	inputs             []textinput.Model
	focusIndex         int
	sending            bool
	errText            string
	resp               responseMsg
	respLines          int
	respRows           []string
	respCursor         int
	respExpanded       []string
	respCursorExpanded int
	viewport           viewport.Model
	fileViewport       viewport.Model
	fileLines          int
	fileOpen           bool
	fileErr            string
	filePath           string
	previewAnimActive  bool
	previewAnimDir     int
	previewAnimWidth   int
	previewFullWidth   int
	width              int
	height             int
	title              lipgloss.Style
	focused            lipgloss.Style
	help               lipgloss.Style
	responseS          lipgloss.Style
	fileS              lipgloss.Style
	hasResponse        bool
	respScrollDrag     bool
}

const (
	fieldURL = iota
	fieldQuery
	fieldHeaders
	fieldBody
	fieldSend
	fieldResponse
)

func main() {
	m := newModel()
	if _, err := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion()).Run(); err != nil {
		fmt.Println("TUI error:", err)
	}
}

func newModel() model {
	inputs := make([]textinput.Model, 4)
	placeholderStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Italic(true).
		Faint(true)

	urlInput := textinput.New()
	urlInput.Placeholder = "http://localhost:8080/api/batches"
	urlInput.Prompt = "URL: "
	urlInput.PlaceholderStyle = placeholderStyle
	urlInput.Focus()
	urlInput.SetValue("http://localhost:8080/api/batches")

	queryInput := textinput.New()
	queryInput.Placeholder = "start=20260120-100000&end=20260120-120000"
	queryInput.Prompt = "Query: "
	queryInput.PlaceholderStyle = placeholderStyle
	queryInput.SetValue("start=20260120-100000&end=20260120-120000")

	headersInput := textinput.New()
	headersInput.Placeholder = "Accept: application/json"
	headersInput.Prompt = "Headers: "
	headersInput.PlaceholderStyle = placeholderStyle
	headersInput.SetValue("Accept: application/json")

	bodyInput := textinput.New()
	bodyInput.Placeholder = "{\"name\":\"demo\"}"
	bodyInput.Prompt = "Body: "
	bodyInput.PlaceholderStyle = placeholderStyle
	bodyInput.SetValue("{\"name\":\"demo\"}")

	inputs[fieldURL] = urlInput
	inputs[fieldQuery] = queryInput
	inputs[fieldHeaders] = headersInput
	inputs[fieldBody] = bodyInput

	vp := viewport.New(0, 0)
	defaultResp := "Response will appear here."
	vp.SetContent(defaultResp)
	fileVP := viewport.New(0, 0)
	fileVP.SetContent("Select a csv_files entry and press Enter.")

	m := model{
		methods:   []string{"GET", "POST", "PUT", "DELETE"},
		methodIdx: 0,
		inputs:    inputs,
		focused:   lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true),
		title:     lipgloss.NewStyle().Bold(true),
		help:      lipgloss.NewStyle().Foreground(lipgloss.Color("241")),
		responseS: lipgloss.NewStyle().
			Background(lipgloss.Color("235")).
			Foreground(lipgloss.Color("252")).
			Padding(0, 1),
		fileS: lipgloss.NewStyle().
			Background(lipgloss.Color("52")).
			Foreground(lipgloss.Color("254")).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("88")).
			Padding(0, 1),
		viewport:     vp,
		fileViewport: fileVP,
		respLines:    countLines(defaultResp),
		respRows:     strings.Split(defaultResp, "\n"),
		fileLines:    countLines("Select a csv_files entry and press Enter."),
		hasResponse:  false,
	}
	m.respRows = strings.Split(defaultResp, "\n")
	m.rebuildExpanded()
	return m
}

func (m model) Init() tea.Cmd {
	return textinput.Blink
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.refreshViewportSizes()
		m.rebuildExpanded()
		return m, tea.ClearScreen
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "tab", "down", "ctrl+n", "j":
			if m.focusIndex == fieldResponse {
				if m.fileOpen {
					m.fileViewport.LineDown(1)
					m.rebuildExpanded()
					return m, nil
				}
				m.moveResponseCursor(1)
				return m, nil
			}
			m.focusIndex = (m.focusIndex + 1) % (len(m.inputs) + 2)
		case "shift+tab", "up", "ctrl+p", "k":
			if m.focusIndex == fieldResponse {
				if m.fileOpen {
					m.fileViewport.LineUp(1)
					m.rebuildExpanded()
					return m, nil
				}
				m.moveResponseCursor(-1)
				return m, nil
			}
			m.focusIndex--
			if m.focusIndex < 0 {
				m.focusIndex = len(m.inputs) + 1
			}
		case "esc":
			if m.focusIndex == fieldResponse {
				if m.fileOpen {
					return m, m.startPreviewClose()
				}
				m.focusIndex = fieldSend
				return m, nil
			}
		case "left":
			if m.focusIndex == fieldSend || m.focusIndex == fieldResponse {
				return m, nil
			}
			if m.focusIndex == fieldURL && m.methodIdx > 0 {
				m.methodIdx--
			}
		case "right":
			if m.focusIndex == fieldSend || m.focusIndex == fieldResponse {
				return m, nil
			}
			if m.focusIndex == fieldURL && m.methodIdx < len(m.methods)-1 {
				m.methodIdx++
			}
		case "pgdown":
			if m.focusIndex == fieldResponse {
				if m.fileOpen {
					m.fileViewport.PageDown()
					m.rebuildExpanded()
					return m, nil
				}
				m.viewport.PageDown()
				m.rebuildExpanded()
				return m, nil
			}
		case "pgup":
			if m.focusIndex == fieldResponse {
				if m.fileOpen {
					m.fileViewport.PageUp()
					m.rebuildExpanded()
					return m, nil
				}
				m.viewport.PageUp()
				m.rebuildExpanded()
				return m, nil
			}
		case " ":
			if m.focusIndex == fieldResponse {
				if m.fileOpen {
					return m, m.startPreviewClose()
				}
				line := m.currentResponseLine()
				if path := extractCSVPathFromLine(line); path != "" {
					return m, m.startPreviewOpen(path)
				}
				return m, nil
			}
		case "enter":
			if m.focusIndex == fieldSend && !m.sending {
				m.sending = true
				m.errText = ""
				return m, sendRequestCmd(m)
			}
			if m.focusIndex == fieldResponse {
				line := m.currentResponseLine()
				if path := extractCSVPathFromLine(line); path != "" {
					return m, m.startPreviewOpen(path)
				}
				return m, nil
			}
		}
	case tea.MouseMsg:
		if !m.hasResponse || m.focusIndex != fieldResponse {
			break
		}
		if !withinResponseBox(m, msg.Y) {
			if msg.Action == tea.MouseActionRelease {
				m.respScrollDrag = false
			}
			break
		}
		switch {
		case msg.Button == tea.MouseButtonWheelDown:
			if m.fileOpen {
				m.fileViewport.LineDown(3)
				m.rebuildExpanded()
				return m, nil
			}
			m.viewport.LineDown(3)
			m.rebuildExpanded()
			return m, nil
		case msg.Button == tea.MouseButtonWheelUp:
			if m.fileOpen {
				m.fileViewport.LineUp(3)
				m.rebuildExpanded()
				return m, nil
			}
			m.viewport.LineUp(3)
			m.rebuildExpanded()
			return m, nil
		case msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress:
			if onResponseScrollbar(m, msg.X) {
				m.respScrollDrag = true
				m.scrollResponseToY(msg.Y)
				return m, nil
			}
		case msg.Action == tea.MouseActionMotion && m.respScrollDrag:
			m.scrollResponseToY(msg.Y)
			return m, nil
		case msg.Action == tea.MouseActionRelease:
			m.respScrollDrag = false
		}
	case previewAnimMsg:
		if !m.previewAnimActive {
			return m, nil
		}
		step := max(2, m.previewFullWidth/10)
		m.previewAnimWidth += step * m.previewAnimDir
		if m.previewAnimDir > 0 && m.previewAnimWidth >= m.previewFullWidth {
			m.previewAnimWidth = m.previewFullWidth
			m.previewAnimActive = false
		}
		if m.previewAnimDir < 0 && m.previewAnimWidth <= 0 {
			m.previewAnimWidth = 0
			m.previewAnimActive = false
			m.fileOpen = false
			m.filePath = ""
			m.fileViewport.SetContent("")
			m.fileLines = 0
		}
		m.rebuildExpanded()
		if m.previewAnimActive {
			return m, previewAnimTick()
		}
		return m, nil
	case responseMsg:
		m.sending = false
		m.resp = msg
		if msg.err != nil {
			m.errText = msg.err.Error()
		} else {
			m.errText = ""
		}
		rendered := formatResponse(msg)
		m.respLines = countLines(rendered)
		m.respRows = strings.Split(rendered, "\n")
		m.respCursor = 0
		m.viewport.SetYOffset(0)
		m.hasResponse = true
		m.refreshViewportSizes()
		m.fileOpen = false
		m.previewAnimActive = false
		m.previewAnimWidth = 0
		m.previewFullWidth = 0
		m.filePath = ""
		m.fileViewport.SetContent("")
		m.fileLines = 0
		m.rebuildExpanded()
		return m, nil
	}

	cmds := make([]tea.Cmd, 0, len(m.inputs))
	for i := range m.inputs {
		if i == m.focusIndex {
			m.inputs[i].Focus()
			m.inputs[i].PromptStyle = m.focused
			m.inputs[i].TextStyle = m.focused
		} else {
			m.inputs[i].Blur()
			m.inputs[i].PromptStyle = lipgloss.NewStyle()
			m.inputs[i].TextStyle = lipgloss.NewStyle()
		}
		var cmd tea.Cmd
		m.inputs[i], cmd = m.inputs[i].Update(msg)
		cmds = append(cmds, cmd)
	}

	m.viewport, _ = m.viewport.Update(msg)
	m.fileViewport, _ = m.fileViewport.Update(msg)
	return m, tea.Batch(cmds...)
}

func (m model) View() string {
	methods := make([]string, 0, len(m.methods))
	for i, method := range m.methods {
		if i == m.methodIdx {
			methods = append(methods, m.focused.Render("["+method+"]"))
		} else {
			methods = append(methods, "["+method+"]")
		}
	}

	sendLabel := "Send"
	if m.sending {
		sendLabel = "Sending..."
	}
	if m.focusIndex == fieldSend {
		sendLabel = m.focused.Render("[" + sendLabel + "]")
	} else {
		sendLabel = "[" + sendLabel + "]"
	}
	responseLabel := "Response:"
	if m.focusIndex == fieldResponse {
		responseLabel = m.focused.Render(responseLabel)
	}

	lines := []string{
		m.title.Render("API Request Builder"),
		m.help.Render("Tab/↑/↓: move • Space: preview • PgUp/PgDn or j/k: scroll • q: quit"),
		"",
		"Method: " + strings.Join(methods, " "),
		m.inputs[fieldURL].View(),
		m.inputs[fieldQuery].View(),
		m.inputs[fieldHeaders].View(),
		m.inputs[fieldBody].View(),
		"",
		"Action: " + sendLabel,
		"",
		responseLabel,
		renderResponseBox(m),
	}

	if m.errText != "" {
		lines = append(lines, "", "Error: "+m.errText)
	}

	return strings.Join(lines, "\n")
}

func (m *model) refreshViewportSizes() {
	if m.width == 0 || m.height == 0 {
		return
	}
	m.viewport.Width = max(24, m.width-4)
	extraBorder := 0
	if m.hasResponse {
		extraBorder = 2
	}
	extraFooter := 0
	if m.errText != "" {
		extraFooter = 2
	}
	availableHeight := m.height - responseBoxTop() - extraBorder - extraFooter
	m.viewport.Height = max(3, availableHeight)
	m.fileViewport.Width = max(10, m.viewport.Width-4)
	m.fileViewport.Height = min(8, max(3, m.viewport.Height/2))
}

func sendRequestCmd(m model) tea.Cmd {
	method := m.methods[m.methodIdx]
	rawURL := strings.TrimSpace(m.inputs[fieldURL].Value())
	query := strings.TrimSpace(m.inputs[fieldQuery].Value())
	headersRaw := strings.TrimSpace(m.inputs[fieldHeaders].Value())
	body := strings.TrimSpace(m.inputs[fieldBody].Value())

	return func() tea.Msg {
		if rawURL == "" {
			return responseMsg{err: fmt.Errorf("URL is required")}
		}

		parsedURL, err := url.Parse(rawURL)
		if err != nil {
			return responseMsg{err: fmt.Errorf("invalid URL: %w", err)}
		}
		if query != "" {
			parsedQuery, parseErr := url.ParseQuery(query)
			if parseErr != nil {
				return responseMsg{err: fmt.Errorf("invalid query: %w", parseErr)}
			}
			current := parsedURL.Query()
			for key, values := range parsedQuery {
				for _, value := range values {
					current.Add(key, value)
				}
			}
			parsedURL.RawQuery = current.Encode()
		}

		var bodyReader *bytes.Reader
		if body != "" && method != "GET" {
			bodyReader = bytes.NewReader([]byte(body))
		} else {
			bodyReader = bytes.NewReader(nil)
		}

		req, err := http.NewRequest(method, parsedURL.String(), bodyReader)
		if err != nil {
			return responseMsg{err: err}
		}

		for _, line := range strings.Split(headersRaw, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			parts := strings.SplitN(line, ":", 2)
			if len(parts) != 2 {
				continue
			}
			req.Header.Set(strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]))
		}
		if body != "" && req.Header.Get("Content-Type") == "" && method != "GET" {
			req.Header.Set("Content-Type", "application/json")
		}

		client := &http.Client{Timeout: 10 * time.Second}
		start := time.Now()
		resp, err := client.Do(req)
		if err != nil {
			return responseMsg{err: err}
		}
		defer resp.Body.Close()

		buf := new(bytes.Buffer)
		if _, err := buf.ReadFrom(resp.Body); err != nil {
			return responseMsg{err: err}
		}

		return responseMsg{
			status:   resp.Status,
			body:     buf.String(),
			duration: time.Since(start),
		}
	}
}

func formatResponse(msg responseMsg) string {
	if msg.err != nil {
		return ""
	}
	body := msg.body
	if len(body) == 0 {
		body = "(empty body)"
	}
	return fmt.Sprintf("Status: %s\nTime: %s\n\n%s", msg.status, msg.duration.Round(time.Millisecond), body)
}

func (m *model) moveResponseCursor(delta int) {
	if len(m.respRows) == 0 {
		return
	}
	next := m.respCursor + delta
	if next < 0 {
		m.focusIndex = fieldSend
		return
	}
	if next >= len(m.respRows) {
		m.focusIndex = fieldURL
		return
	}
	m.respCursor = next
	m.rebuildExpanded()
}

func (m *model) rebuildExpanded() {
	expanded := make([]string, 0, len(m.respRows))
	cursorExpanded := 0
	var previewLines []string
	var previewWidth int
	if m.fileOpen || m.previewAnimActive {
		fullLines, fullWidth := buildPreviewLines(m)
		m.previewFullWidth = fullWidth
		previewWidth = fullWidth
		if m.previewAnimActive {
			previewWidth = clamp(m.previewAnimWidth, 0, fullWidth)
		}
		previewLines = truncateLines(fullLines, previewWidth)
	}
	for i, line := range m.respRows {
		if i == m.respCursor {
			cursorExpanded = len(expanded)
		}
		expanded = append(expanded, line)
		if (m.fileOpen || m.previewAnimActive) && m.filePath != "" && extractCSVPathFromLine(line) == m.filePath {
			expanded = append(expanded, previewLines...)
		}
	}
	m.respExpanded = expanded
	m.respCursorExpanded = cursorExpanded
	m.viewport.SetContent(strings.Join(expanded, "\n"))
	m.respLines = len(expanded)
	if m.respCursorExpanded < m.viewport.YOffset {
		m.viewport.SetYOffset(m.respCursorExpanded)
	} else if m.respCursorExpanded >= m.viewport.YOffset+m.viewport.Height {
		m.viewport.SetYOffset(m.respCursorExpanded - m.viewport.Height + 1)
	}
}

func (m *model) startPreviewOpen(path string) tea.Cmd {
	content, err := readFilePreview(path, 4000, 200)
	if err != nil {
		m.fileErr = err.Error()
		m.fileViewport.SetContent("File error: " + m.fileErr)
		m.fileLines = countLines("File error: " + m.fileErr)
	} else {
		m.fileErr = ""
		m.fileViewport.SetContent(content)
		m.fileLines = countLines(content)
	}
	m.fileViewport.SetYOffset(0)
	m.fileOpen = true
	m.filePath = path
	m.previewAnimActive = true
	m.previewAnimDir = 1
	m.previewAnimWidth = 0
	m.rebuildExpanded()
	return previewAnimTick()
}

func (m *model) startPreviewClose() tea.Cmd {
	m.previewAnimActive = true
	m.previewAnimDir = -1
	if m.previewAnimWidth == 0 {
		m.previewAnimWidth = m.previewFullWidth
	}
	m.rebuildExpanded()
	return previewAnimTick()
}

func (m model) currentResponseLine() string {
	if len(m.respRows) == 0 || m.respCursor < 0 || m.respCursor >= len(m.respRows) {
		return ""
	}
	return m.respRows[m.respCursor]
}

func extractCSVPathFromLine(line string) string {
	line = strings.TrimSpace(line)
	if !strings.Contains(line, "\"name\"") && !strings.Contains(line, "\"num\"") && !strings.Contains(line, "\"time\"") {
		return ""
	}
	colon := strings.Index(line, ":")
	if colon == -1 {
		return ""
	}
	rest := strings.TrimSpace(line[colon+1:])
	rest = strings.Trim(rest, "\",")
	rest = strings.Trim(rest, "\"")
	return rest
}

func readFilePreview(path string, maxBytes int, maxLines int) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	var buf bytes.Buffer
	scanner := bufio.NewScanner(f)
	lineCount := 0
	for scanner.Scan() {
		line := scanner.Text()
		buf.WriteString(line)
		buf.WriteByte('\n')
		lineCount++
		if lineCount >= maxLines || buf.Len() >= maxBytes {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func renderViewportWithScrollbar(v viewport.Model, totalLines int) string {
	if totalLines <= v.Height || v.Height == 0 {
		return v.View()
	}
	lines := strings.Split(v.View(), "\n")
	height := v.Height
	if len(lines) < height {
		for i := len(lines); i < height; i++ {
			lines = append(lines, "")
		}
	}
	thumbSize := max(1, (height*height)/totalLines)
	if thumbSize > height {
		thumbSize = height
	}
	thumbTop := int(float64(height-thumbSize) * v.ScrollPercent())
	scrollLines := make([]string, 0, height)
	for i := 0; i < height; i++ {
		bar := "|"
		if i >= thumbTop && i < thumbTop+thumbSize {
			bar = "#"
		}
		scrollLines = append(scrollLines, lines[i]+" "+bar)
	}
	return strings.Join(scrollLines, "\n")
}

func renderResponseWithCursor(m model) string {
	if len(m.respExpanded) == 0 {
		return ""
	}
	start := m.viewport.YOffset
	end := min(start+m.viewport.Height, len(m.respExpanded))
	visible := make([]string, 0, end-start)
	highlight := lipgloss.NewStyle().Background(lipgloss.Color("237")).Foreground(lipgloss.Color("255"))
	width := m.viewport.Width
	for i := start; i < end; i++ {
		line := ansi.Truncate(m.respExpanded[i], width, "")
		if m.focusIndex == fieldResponse && i == m.respCursorExpanded {
			line = highlight.Render(line)
		}
		visible = append(visible, line)
	}
	for len(visible) < m.viewport.Height {
		visible = append(visible, "")
	}
	return renderLinesWithScrollbar(visible, m.viewport, len(m.respExpanded))
}

func renderResponseBox(m model) string {
	content := renderResponseWithCursor(m)
	if !m.hasResponse {
		return content
	}
	return m.responseS.Copy().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("239")).
		Render(content)
}

func responseBoxTop() int {
	// Lines before the response box in View():
	// title, help, blank, method, URL, Query, Headers, Body, blank, action, blank, responseLabel
	return 12
}

func withinResponseBox(m model, y int) bool {
	top, _, _, height := responseBoxMetrics(m)
	return y >= top && y < top+height
}

func responseBoxMetrics(m model) (top, left, width, height int) {
	top = responseBoxTop()
	left = 0
	contentWidth := m.viewport.Width + 2 // space + bar
	width = contentWidth + 2             // padding left/right
	if m.hasResponse {
		width += 2
		height = m.viewport.Height + 2 // border
	} else {
		height = m.viewport.Height
	}
	return
}

func onResponseScrollbar(m model, x int) bool {
	_, left, width, _ := responseBoxMetrics(m)
	scrollbarX := left + width - 1
	return x >= scrollbarX-1
}

func (m *model) scrollResponseToY(y int) {
	top, _, _, _ := responseBoxMetrics(*m)
	contentTop := top
	if m.hasResponse {
		contentTop++
	}
	row := y - contentTop
	if row < 0 {
		row = 0
	}
	if row >= m.viewport.Height {
		row = m.viewport.Height - 1
	}
	maxOffset := max(len(m.respExpanded)-m.viewport.Height, 0)
	if maxOffset == 0 {
		return
	}
	ratio := float64(row) / float64(max(m.viewport.Height-1, 1))
	target := int(float64(maxOffset) * ratio)
	m.viewport.SetYOffset(target)
	m.rebuildExpanded()
}

func renderLinesWithScrollbar(lines []string, v viewport.Model, totalLines int) string {
	if totalLines <= v.Height || v.Height == 0 {
		return strings.Join(lines, "\n")
	}
	height := v.Height
	thumbSize := max(1, (height*height)/totalLines)
	if thumbSize > height {
		thumbSize = height
	}
	thumbTop := int(float64(height-thumbSize) * v.ScrollPercent())
	scrollLines := make([]string, 0, height)
	for i := 0; i < height; i++ {
		bar := "|"
		if i >= thumbTop && i < thumbTop+thumbSize {
			bar = "#"
		}
		scrollLines = append(scrollLines, lines[i]+" "+bar)
	}
	return strings.Join(scrollLines, "\n")
}

func buildPreviewLines(m *model) ([]string, int) {
	content := renderViewportWithScrollbar(m.fileViewport, m.fileLines)
	block := m.fileS.Render(content)
	lines := strings.Split(block, "\n")
	width := 0
	for _, line := range lines {
		if w := lipgloss.Width(line); w > width {
			width = w
		}
	}
	return lines, width
}

func truncateLines(lines []string, width int) []string {
	if width <= 0 {
		out := make([]string, len(lines))
		for i := range out {
			out[i] = ""
		}
		return out
	}
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		out = append(out, ansi.Truncate(line, width, ""))
	}
	return out
}

func previewAnimTick() tea.Cmd {
	return tea.Tick(16*time.Millisecond, func(time.Time) tea.Msg {
		return previewAnimMsg{}
	})
}

func clamp(v, low, high int) int {
	if v < low {
		return low
	}
	if v > high {
		return high
	}
	return v
}

func countLines(s string) int {
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
