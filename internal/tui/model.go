package tui

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/atotto/clipboard"
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

// analysisResult is sent when single issue LLM analysis completes
type analysisResult struct {
	issueTitle string
	guidance   *llm.Guidance
	err        error
}

// resourceAnalysisResult is sent when a resource group analysis completes
type resourceAnalysisResult struct {
	key       string
	issues    []Issue
	guidances []*llm.Guidance
	err       error
}

// Model is the bubbletea model for aura live TUI
type Model struct {
	client           *client.Client
	snapshot         *model.ClusterSnapshot
	issues           []Issue
	resolved         []ResolvedIssue
	guidance         map[string]*llm.Guidance
	analyzing        map[string]bool
	status           status
	probeCount       int
	lastProbe        time.Time
	nextProbe        int
	errors           []string
	probeTimeMs      int64
	analyzer         llm.Analyzer
	configManager    *config.Manager
	llmConfigured    bool
	viewMode         string
	copyConfirm      string
	copyConfirmIndex int
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
	Title        string
	ResourceType string
	Resource     string
	ResolvedAt   time.Time
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
				m.guidance = make(map[string]*llm.Guidance)
				m.analyzing = make(map[string]bool)
				m.copyConfirm = ""
				return m, m.analyzeAll()
			}

		case "esc", "b":
			m.viewMode = "main"
			m.copyConfirm = ""
			return m, nil

		case "1", "2", "3", "4", "5", "6", "7", "8", "9":
			if m.viewMode == "analysis" {
				idx, _ := strconv.Atoi(msg.String())
				idx-- // zero based
				if idx < len(m.issues) {
					issue := m.issues[idx]
					key := issue.Title + issue.Resource
					if g, ok := m.guidance[key]; ok && g.Command != "" {
						_ = clipboard.WriteAll(g.Command)
						m.copyConfirm = "✓ copied to clipboard"
						m.copyConfirmIndex = idx
					}
				}
			}
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

		// auto analyze new issues grouped by resource
		if m.analyzer != nil {
			newIssues := findNewIssues(m.issues, prevIssues)
			if len(newIssues) > 0 {
				groups := groupByResource(newIssues)
				var cmds []tea.Cmd
				for key, groupIssues := range groups {
					for _, issue := range groupIssues {
						m.analyzing[issue.Title+issue.Resource] = true
					}
					cmds = append(cmds, m.analyzeResourceGroup(key, groupIssues))
				}
				if len(cmds) > 0 {
					return m, tea.Batch(cmds...)
				}
			}
		}

	case resourceAnalysisResult:
		for _, issue := range msg.issues {
			delete(m.analyzing, issue.Title+issue.Resource)
		}

		if msg.err == nil && len(msg.guidances) > 0 {
			for _, g := range msg.guidances {
				for _, issue := range msg.issues {
					if strings.EqualFold(g.Issue, issue.Title) ||
						(g.Issue == "" && len(msg.issues) == 1) {
						m.guidance[issue.Title+issue.Resource] = g
						break
					}
				}
			}
		}

		if m.viewMode == "analysis" {
			return m, m.analyzeAll()
		}
	}

	return m, nil
}

// groupByResource groups issues by resource key
func groupByResource(issues []Issue) map[string][]Issue {
	groups := make(map[string][]Issue)
	for _, issue := range issues {
		key := resourceKey(issue)
		groups[key] = append(groups[key], issue)
	}
	return groups
}

// resourceKey generates a unique key for a resource
func resourceKey(issue Issue) string {
	return issue.ResourceType + "/" + issue.Resource + "/" + issue.Namespace
}

// analyzeAll groups all unanalyzed issues by resource and analyzes in parallel
func (m *Model) analyzeAll() tea.Cmd {
	if len(m.issues) == 0 || m.analyzer == nil {
		return nil
	}

	// find unanalyzed issues
	var unanalyzed []Issue
	for _, issue := range m.issues {
		key := issue.Title + issue.Resource
		if _, analyzed := m.guidance[key]; !analyzed {
			if !m.analyzing[key] {
				unanalyzed = append(unanalyzed, issue)
			}
		}
	}

	if len(unanalyzed) == 0 {
		return nil
	}

	// group by resource
	groups := groupByResource(unanalyzed)

	var cmds []tea.Cmd
	for key, groupIssues := range groups {
		for _, issue := range groupIssues {
			m.analyzing[issue.Title+issue.Resource] = true
		}
		cmds = append(cmds, m.analyzeResourceGroup(key, groupIssues))
	}

	if len(cmds) == 0 {
		return nil
	}

	return tea.Batch(cmds...)
}

// analyzeResourceGroup analyzes all issues for one resource in one LLM call
func (m *Model) analyzeResourceGroup(key string, issues []Issue) tea.Cmd {
	analyzer := m.analyzer
	c := m.client
	snapshot := m.snapshot

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		if len(issues) == 0 {
			return nil
		}

		// build context using first issue resource info
		first := issues[0]
		ctxBuilder := auracontext.New(c)
		ic, err := ctxBuilder.Build(ctx, snapshot,
			first.Resource, first.Namespace, first.ResourceType, first.Title)
		if err != nil {
			return resourceAnalysisResult{
				key: key, issues: issues, err: err,
			}
		}

		// build LLM context with all issues for this resource
		llmCtx := &llm.IssueContext{
			ResourceName:      ic.ResourceName,
			ResourceNamespace: ic.ResourceNamespace,
			ResourceKind:      ic.ResourceKind,
			IssueTitle:        first.Title,
			IssueSeverity:     first.Severity,
			Identifiers:       ic.Identifiers,
			Events:            ic.Events,
			Logs:              ic.Logs,
			NodeState:         ic.NodeState,
			ClusterName:       ic.ClusterName,
		}

		// add all issues for this resource
		for _, issue := range issues {
			llmCtx.Issues = append(llmCtx.Issues, llm.IssueInput{
				Title:    issue.Title,
				Severity: issue.Severity,
			})
		}

		guidances, err := analyzer.AnalyzeMultiple(ctx, llmCtx)
		return resourceAnalysisResult{
			key:       key,
			issues:    issues,
			guidances: guidances,
			err:       err,
		}
	}
}

// filterBySeverity returns issues of a specific severity
func filterBySeverity(issues []Issue, severity string) []Issue {
	var filtered []Issue
	for _, i := range issues {
		if i.Severity == severity {
			filtered = append(filtered, i)
		}
	}
	return filtered
}

// findNewIssues returns issues not seen in previous probe
func findNewIssues(current, previous []Issue) []Issue {
	prevKeys := make(map[string]bool)
	for _, p := range previous {
		prevKeys[p.ResourceType+p.Resource+p.Title] = true
	}

	var newIssues []Issue
	for _, c := range current {
		if !prevKeys[c.ResourceType+c.Resource+c.Title] {
			newIssues = append(newIssues, c)
		}
	}
	return newIssues
}

// View renders the current state
func (m Model) View() string {
	return render(m)
}
