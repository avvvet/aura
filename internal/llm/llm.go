package llm

import "context"

// Analyzer defines the interface all LLM providers must implement
type Analyzer interface {
	Analyze(ctx context.Context, ic *IssueContext) (*Guidance, error)
	Name() string
}

// IssueContext is the rich context sent to the LLM
type IssueContext struct {
	ResourceName      string
	ResourceNamespace string
	ResourceKind      string
	IssueTitle        string
	IssueSeverity     string
	Identifiers       map[string]string
	Events            []string
	Logs              []string
	NodeState         string
	ClusterName       string
}

// Guidance is the structured response from the LLM
type Guidance struct {
	Type           string `json:"type"`
	RootCause      string `json:"root_cause"`
	FixExplanation string `json:"fix_explanation"`
	Command        string `json:"command"`
	WatchFor       string `json:"watch_for"`
	Risk           string `json:"risk"`
	Confidence     string `json:"confidence"`
}
