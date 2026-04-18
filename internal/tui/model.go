package tui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/avvvet/aura/internal/client"
	"github.com/avvvet/aura/internal/config"
	auracontext "github.com/avvvet/aura/internal/context"
	"github.com/avvvet/aura/internal/llm"
	"github.com/avvvet/aura/internal/model"
)

const probeInterval = 30

type status int

const (
	statusBooting status = iota
	statusProbing
	statusHealthy
	statusIssues
	statusError
)

// analysisResult is sent when LLM analysis completes
type analysisResult struct {
	issueTitle string
	guidance   *llm.Guidance
	err        error
}

// Model is the bubbletea model for aura live TUI
type Model struct {
	client        *client.Client
	snapshot      *model.ClusterSnapshot
	issues        []Issue
	resolved      []ResolvedIssue
	guidance      map[string]*llm.Guidance // key = issue title
	analyzing     map[string]bool          // issues currently being analyzed
	status        status
	probeCount    int
	lastProbe     time.Time
	nextProbe     int
	errors        []string
	probeTimeMs   int64
	analyzer      llm.Analyzer
	configManager *config.Manager
	llmConfigured bool
	viewMode      string // "main" or "analysis"
}

// Issue represents a detected cluster issue
type Issue struct {
	Severity     string
	ResourceType string
	Title        string
	Resource     string
	Namespace    string
	Meta         string
	Command      string
	DetectedAt   time.Time
}

// ResolvedIssue represents an issue that was fixed
type ResolvedIssue struct {
	Title      string
	ResolvedAt time.Time
}

// New creates a new TUI model
func New(c *client.Client, cfgManager *config.Manager) Model {
	m := Model{
		client:        c,
		status:        statusBooting,
		nextProbe:     probeInterval,
		guidance:      make(map[string]*llm.Guidance),
		analyzing:     make(map[string]bool),
		configManager: cfgManager,
		viewMode:      "main",
	}

	// load LLM config
	cfg, err := cfgManager.LoadConfig()
	if err == nil && cfg.LLMProvider != "" {
		apiKey, _ := cfgManager.LoadAPIKey()
		m.analyzer = buildAnalyzer(cfg, apiKey)
		m.llmConfigured = true
	}

	return m
}

// buildAnalyzer creates the right analyzer based on config
func buildAnalyzer(cfg *config.Config, apiKey string) llm.Analyzer {
	switch cfg.LLMProvider {
	case config.ProviderOllama:
		return llm.NewOllamaAnalyzer(cfg.LLMEndpoint, cfg.LLMModel)
	case config.ProviderOpenAI:
		return llm.NewOpenAIAnalyzer(cfg.LLMEndpoint, cfg.LLMModel, apiKey)
	case config.ProviderAnthropic:
		return llm.NewAnthropicAnalyzer(cfg.LLMEndpoint, cfg.LLMModel, apiKey)
	default:
		return nil
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
			if m.configManager != nil {
				m.configManager.Close()
			}
			return m, tea.Quit

		case "a":
			if m.viewMode == "main" {
				m.viewMode = "analysis"
				if m.analyzer != nil && len(m.issues) > 0 {
					return m, m.analyzeAll()
				}
				return m, nil
			}
			if m.viewMode == "analysis" && m.analyzer != nil && len(m.issues) > 0 {
				return m, m.analyzeAll()
			}

		case "esc", "b":
			m.viewMode = "main"
			return m, nil
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

		prevIssues := m.issues
		m.snapshot = msg.snapshot
		m.probeCount++
		m.lastProbe = time.Now()
		m.probeTimeMs = msg.durationMs

		m.issues = detectIssues(m.snapshot)
		m.resolved = detectResolved(m.issues, m.resolved, prevIssues)

		if len(m.issues) == 0 {
			m.status = statusHealthy
		} else {
			m.status = statusIssues
		}

		// auto analyze new issues if LLM configured
		if m.analyzer != nil {
			newIssues := findNewIssues(m.issues, prevIssues)
			if len(newIssues) > 0 {
				return m, m.analyzeIssues(newIssues)
			}
		}

	case analysisResult:
		if msg.err == nil && msg.guidance != nil {
			m.guidance[msg.issueTitle] = msg.guidance
		}
		delete(m.analyzing, msg.issueTitle)
	}

	return m, nil
}

// analyzeAll triggers analysis for all current issues
func (m *Model) analyzeAll() tea.Cmd {
	return m.analyzeIssues(m.issues)
}

// analyzeIssues triggers LLM analysis for a list of issues
func (m *Model) analyzeIssues(issues []Issue) tea.Cmd {
	var cmds []tea.Cmd
	for _, issue := range issues {
		if !m.analyzing[issue.Title] {
			m.analyzing[issue.Title] = true
			cmds = append(cmds, m.analyzeIssue(issue))
		}
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}

// analyzeIssue runs LLM analysis for a single issue
func (m *Model) analyzeIssue(issue Issue) tea.Cmd {
	analyzer := m.analyzer
	client := m.client
	snapshot := m.snapshot

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		ctxBuilder := auracontext.New(client)
		ic, err := ctxBuilder.Build(ctx, snapshot, issue.Resource, issue.Namespace, issue.Resource)
		if err != nil {
			return analysisResult{issueTitle: issue.Title, err: err}
		}

		llmCtx := &llm.IssueContext{
			ResourceName:      ic.ResourceName,
			ResourceNamespace: ic.ResourceNamespace,
			ResourceKind:      issue.Resource,
			IssueTitle:        issue.Title,
			IssueSeverity:     issue.Severity,
			Events:            ic.Events,
			Logs:              ic.Logs,
			NodeState:         ic.NodeState,
			ClusterName:       ic.ClusterName,
		}

		guidance, err := analyzer.Analyze(ctx, llmCtx)
		return analysisResult{
			issueTitle: issue.Title,
			guidance:   guidance,
			err:        err,
		}
	}
}

// findNewIssues returns issues that weren't in the previous probe
func findNewIssues(current, previous []Issue) []Issue {
	prevTitles := make(map[string]bool)
	for _, p := range previous {
		prevTitles[p.Title] = true
	}

	var newIssues []Issue
	for _, c := range current {
		if !prevTitles[c.Title] {
			newIssues = append(newIssues, c)
		}
	}
	return newIssues
}

// View renders the current state
func (m Model) View() string {
	return render(m)
}
