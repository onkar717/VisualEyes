// Package tui implements the Bubbletea interactive dashboard for veye watch.
package tui

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/onkar717/visual-eyes/veye/internal/client"
	"github.com/onkar717/visual-eyes/veye/internal/styles"
)

// view is one of the three main panes.
type view int

const (
	viewAlerts view = iota
	viewLogs
	viewRCA
)

// tickMsg is sent on each refresh tick.
type tickMsg time.Time

// dataMsg carries a fresh backend snapshot.
type dataMsg struct {
	snap   *client.Snapshot
	k8s    *client.K8sMetrics
	alerts []client.Alert
	logs   []client.PodLog
	err    error
}

// rcaMsg carries the RCA result for the selected alert.
type rcaMsg struct {
	rca *client.RCAResult
	err error
}

// Model is the Bubbletea application state.
type Model struct {
	api       *client.Client
	width     int
	height    int

	activeView  view
	cursor      int   // selected alert index
	logScroll   int   // log pane scroll offset
	lastRefresh time.Time
	loading     bool

	snap   *client.Snapshot
	k8s    *client.K8sMetrics
	alerts []client.Alert
	logs   []client.PodLog
	rca    *client.RCAResult
	rcaErr error
	err    error
}

// New creates a Model connected to the given client.
func New(api *client.Client) Model {
	return Model{api: api, activeView: viewAlerts, loading: true}
}

// Init kicks off the first data fetch.
func (m Model) Init() tea.Cmd {
	return tea.Batch(fetchData(m.api), tickEvery(15*time.Second))
}

func tickEvery(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func fetchData(api *client.Client) tea.Cmd {
	return func() tea.Msg {
		snap, _ := api.Snapshot()
		k8s, _ := api.K8s()
		alerts, _ := api.Alerts("firing")
		logs, err := api.Logs("", "", "", 200)
		return dataMsg{snap: snap, k8s: k8s, alerts: alerts, logs: logs, err: err}
	}
}

func fetchRCA(api *client.Client, alertID uint) tea.Cmd {
	return func() tea.Msg {
		rca, err := api.RCA(alertID)
		return rcaMsg{rca: rca, err: err}
	}
}

// Update handles all messages and key presses.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height

	case tickMsg:
		m.loading = true
		return m, tea.Batch(fetchData(m.api), tickEvery(15*time.Second))

	case dataMsg:
		m.loading = false
		m.err = msg.err
		m.snap = msg.snap
		m.k8s = msg.k8s
		m.alerts = msg.alerts
		m.logs = msg.logs
		m.lastRefresh = time.Now()
		if m.cursor >= len(m.alerts) && len(m.alerts) > 0 {
			m.cursor = len(m.alerts) - 1
		}

	case rcaMsg:
		m.rca = msg.rca
		m.rcaErr = msg.err

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "a":
		m.activeView = viewAlerts
		m.rca = nil

	case "l":
		m.activeView = viewLogs

	// Navigate alerts
	case "up", "k":
		if m.activeView == viewAlerts && m.cursor > 0 {
			m.cursor--
			m.rca = nil
		} else if m.activeView == viewLogs && m.logScroll > 0 {
			m.logScroll--
		}

	case "down", "j":
		if m.activeView == viewAlerts && m.cursor < len(m.alerts)-1 {
			m.cursor++
			m.rca = nil
		} else if m.activeView == viewLogs {
			m.logScroll++
		}

	// Run RCA for selected alert
	case "r":
		if m.activeView == viewAlerts && len(m.alerts) > 0 {
			a := m.alerts[m.cursor]
			if a.RCAStatus == "done" || a.RCAStatus == "running" || a.RCAStatus == "failed" {
				m.activeView = viewRCA
				m.rca = nil
				return m, fetchRCA(m.api, a.ID)
			}
		}

	// Refresh now
	case "R":
		m.loading = true
		return m, fetchData(m.api)
	}

	return m, nil
}

// View renders the full TUI screen.
func (m Model) View() string {
	if m.width == 0 {
		return "loading…"
	}

	header := m.renderHeader()
	tabs := m.renderTabs()
	body := m.renderBody()
	help := m.renderHelp()

	return lipgloss.JoinVertical(lipgloss.Left, header, tabs, body, help)
}

func (m Model) renderHeader() string {
	status := styles.Good.Render("●  connected")
	if m.err != nil {
		status = styles.Bad.Render("●  " + m.err.Error())
	}

	refresh := styles.Mute.Render("  last refresh " + m.lastRefresh.Format("15:04:05"))
	if m.loading {
		refresh = styles.SevWarning.Render("  ⟳ refreshing…")
	}

	alertCount := ""
	if len(m.alerts) > 0 {
		alertCount = styles.Bad.Render(fmt.Sprintf("  ⚠ %d firing", len(m.alerts)))
	}

	row := lipgloss.JoinHorizontal(lipgloss.Left,
		styles.Banner.Render(" VisualEyes "),
		styles.Mute.Render(" · "),
		status,
		alertCount,
		refresh,
	)
	return styles.Box.Width(m.width - 2).Render(row)
}

func (m Model) renderTabs() string {
	makeTab := func(label string, v view) string {
		if m.activeView == v {
			return lipgloss.NewStyle().
				Foreground(styles.Cyan).Bold(true).
				Padding(0, 2).
				Underline(true).
				Render(label)
		}
		return lipgloss.NewStyle().
			Foreground(styles.Gray).
			Padding(0, 2).
			Render(label)
	}
	tabs := lipgloss.JoinHorizontal(lipgloss.Left,
		makeTab("[a] Alerts", viewAlerts),
		makeTab("[l] Logs", viewLogs),
	)
	if m.activeView == viewRCA {
		tabs += lipgloss.NewStyle().Foreground(styles.Cyan).Bold(true).Padding(0, 2).Underline(true).Render("[r] RCA")
	}
	return "\n" + tabs + "\n"
}

func (m Model) renderBody() string {
	bodyH := m.height - 8
	if bodyH < 4 {
		bodyH = 4
	}
	switch m.activeView {
	case viewLogs:
		return m.renderLogs(bodyH)
	case viewRCA:
		return m.renderRCA(bodyH)
	default:
		return m.renderAlerts(bodyH)
	}
}

func (m Model) renderAlerts(h int) string {
	if len(m.alerts) == 0 {
		return styles.Good.Render("\n  ✓ No firing alerts   system is healthy\n")
	}

	var sb strings.Builder
	// Column header
	sb.WriteString(fmt.Sprintf("  %s%s%s%s\n",
		styles.KeyStyle.Width(8).Render("SEV"),
		styles.KeyStyle.Width(28).Render("RULE"),
		styles.KeyStyle.Width(20).Render("RESOURCE"),
		styles.KeyStyle.Width(18).Render("VALUE / THRESH"),
	))
	sb.WriteString("  " + styles.Mute.Render(strings.Repeat("-", 72)) + "\n")

	for i, a := range m.alerts {
		sev := styles.SeverityBadge(a.Severity)
		rule := truncate(a.RuleName, 27)
		res := truncate(a.ResourceID, 19)
		val := fmt.Sprintf("%.2f / %.2f", a.Value, a.Threshold)

		row := fmt.Sprintf("  %s%s%s%s",
			lipgloss.NewStyle().Width(8).Render(sev),
			lipgloss.NewStyle().Width(28).Render(rule),
			styles.Mute.Width(20).Render(res),
			styles.ValStyle.Width(18).Render(val),
		)

		if i == m.cursor {
			row = styles.Selected.Render("▶") + row[1:]
		}
		sb.WriteString(row + "\n")
	}

	// Detail pane for selected alert
	if m.cursor < len(m.alerts) {
		a := m.alerts[m.cursor]
		sb.WriteString("\n")
		detail := fmt.Sprintf("  %s  %s\n  %s",
			styles.SeverityBadge(a.Severity),
			styles.Title.Render(a.RuleName),
			styles.Mute.Render(a.Message),
		)
		hint := ""
		if a.RCAStatus == "done" {
			hint = styles.Good.Render("  press [r] to view Claude's RCA analysis")
		} else if a.RCAStatus == "pending" || a.RCAStatus == "" {
			hint = styles.Mute.Render("  RCA not yet available (enable ANTHROPIC_API_KEY)")
		}
		if hint != "" {
			detail += "\n" + hint
		}
		sb.WriteString(styles.Box.Width(m.width - 4).Render(detail) + "\n")
	}

	return sb.String()
}

func (m Model) renderLogs(h int) string {
	if len(m.logs) == 0 {
		return styles.Mute.Render("\n  no logs yet   K8s agent ships them automatically\n")
	}

	// Window the log lines.
	start := m.logScroll
	if start >= len(m.logs) {
		start = len(m.logs) - 1
	}
	end := start + h - 2
	if end > len(m.logs) {
		end = len(m.logs)
	}

	var sb strings.Builder
	for _, l := range m.logs[start:end] {
		t, err := time.Parse(time.RFC3339Nano, l.Timestamp)
		ts := l.Timestamp
		if err == nil {
			ts = t.Format("15:04:05")
		}
		stream := styles.Mute.Render("out")
		lineColor := lipgloss.NewStyle()
		if l.Stream == "stderr" {
			stream = styles.Bad.Render("err")
			lineColor = styles.SevWarning
		}
		pod := truncate(fmt.Sprintf("%s/%s", l.Namespace, l.Pod), 32)
		sb.WriteString(fmt.Sprintf("  %s  %s  %s  %s\n",
			styles.Mute.Render(ts), stream,
			styles.SevInfo.Render(pod),
			lineColor.Render(l.Line),
		))
	}
	return sb.String()
}

func (m Model) renderRCA(h int) string {
	if m.rca == nil && m.rcaErr == nil {
		return styles.SevWarning.Render("\n  ⟳ Loading RCA analysis…\n")
	}
	if m.rcaErr != nil {
		return styles.Bad.Render("\n  ✗ " + m.rcaErr.Error() + "\n")
	}

	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString(styles.SectionHeader.Render("  ROOT CAUSE") + "\n\n")
	for _, line := range wrapStr(m.rca.RootCause, m.width-6) {
		sb.WriteString("  " + styles.ValStyle.Render(line) + "\n")
	}
	sb.WriteString("\n")
	sb.WriteString(styles.SectionHeader.Render("  ANALYSIS") + "\n\n")
	for _, line := range wrapStr(m.rca.Explanation, m.width-6) {
		sb.WriteString("  " + line + "\n")
	}

	var cmds []client.FixCommand
	if m.rca.Commands != "" {
		_ = json.Unmarshal([]byte(m.rca.Commands), &cmds)
	}
	if len(cmds) > 0 {
		sb.WriteString("\n")
		sb.WriteString(styles.SectionHeader.Render("  REMEDIATION") + "\n\n")
		for i, cmd := range cmds {
			safe := styles.Good.Render("[auto-safe]")
			if !cmd.IsAutoSafe {
				safe = styles.SevWarning.Render("[manual]   ")
			}
			sb.WriteString(fmt.Sprintf("  [%d] %s %s\n", i+1, safe, styles.ValStyle.Render(cmd.Command)))
		}
	}

	sb.WriteString("\n" + styles.Mute.Render(fmt.Sprintf("  model: %s · %d tokens", m.rca.Model, m.rca.InputTokens)))
	return sb.String()
}

func (m Model) renderHelp() string {
	keys := "  ↑/↓ navigate  [a] alerts  [l] logs  [r] RCA  [R] refresh  [q] quit"
	return styles.HelpBar.Render(keys)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func wrapStr(text string, width int) []string {
	words := strings.Fields(text)
	var lines []string
	line := ""
	for _, w := range words {
		if line == "" {
			line = w
		} else if len(line)+1+len(w) <= width {
			line += " " + w
		} else {
			lines = append(lines, line)
			line = w
		}
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
}

