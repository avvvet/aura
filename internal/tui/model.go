package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/avvvet/aura/internal/client"
	"github.com/avvvet/aura/internal/model"
)

const probeInterval = 30 // seconds

// status represents the current probe state
type status int

const (
	statusBooting status = iota
	statusProbing
	statusHealthy
	statusIssues
	statusError
)

// Model is the bubbletea model for aura live TUI
type Model struct {
	client      *client.Client
	snapshot    *model.ClusterSnapshot
	issues      []Issue
	resolved    []ResolvedIssue
	status      status
	probeCount  int
	lastProbe   time.Time
	nextProbe   int
	errors      []string
	probeTimeMs int64
}

// Issue represents a detected cluster issue
type Issue struct {
	Severity   string // critical, warning, info
	Title      string
	Resource   string
	Namespace  string
	Meta       string
	Command    string
	DetectedAt time.Time
}

// ResolvedIssue represents an issue that was fixed
type ResolvedIssue struct {
	Title      string
	ResolvedAt time.Time
}

// New creates a new TUI model
func New(c *client.Client) Model {
	return Model{
		client:    c,
		status:    statusBooting,
		nextProbe: probeInterval,
	}
}

// Init starts the first probe and ticker
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		probe(m.client),
		tick(),
	)
}

// Update handles all messages
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		}

	case tickMsg:
		m.nextProbe--
		if m.nextProbe <= 0 {
			m.nextProbe = probeInterval
			m.status = statusProbing
			return m, tea.Batch(tick(), probe(m.client))
		}
		return m, tick()

	case probeDone:
		if msg.err != nil {
			m.status = statusError
			m.errors = append(m.errors, msg.err.Error())
			return m, nil
		}

		start := time.Now()
		m.snapshot = msg.snapshot
		m.probeCount++
		m.lastProbe = time.Now()
		m.probeTimeMs = time.Since(start).Milliseconds()

		// detect issues from snapshot
		m.issues = detectIssues(m.snapshot)
		m.resolved = detectResolved(m.issues, m.resolved)

		if len(m.issues) == 0 {
			m.status = statusHealthy
		} else {
			m.status = statusIssues
		}
	}

	return m, nil
}

// View renders the current state
func (m Model) View() string {
	return render(m)
}
