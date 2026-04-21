package llm

import (
	"os"
	"path/filepath"
	"strings"
)

// PromptLoader loads prompt files from ~/.steered/skills/
type PromptLoader struct {
	baseDir string
}

// NewPromptLoader creates a new PromptLoader
func NewPromptLoader() *PromptLoader {
	home, _ := os.UserHomeDir()
	return &PromptLoader{
		baseDir: filepath.Join(home, ".steered", "skills"),
	}
}

// LoadBase loads the base prompt from file or returns default
func (p *PromptLoader) LoadBase() string {
	return p.loadFile("base.md", defaultBasePrompt)
}

// LoadIssue loads resource-specific issue guidance from file
func (p *PromptLoader) LoadIssue(resourceKind string) string {
	filename := filepath.Join("resources", strings.ToLower(resourceKind)+".md")
	return p.loadFile(filename, p.defaultIssuePrompt(resourceKind))
}

// loadFile reads a prompt file or returns fallback
func (p *PromptLoader) loadFile(name, fallback string) string {
	path := filepath.Join(p.baseDir, name)
	data, err := os.ReadFile(path)
	if err != nil {
		return fallback
	}
	return string(data)
}

// defaultIssuePrompt returns built-in default for each resource kind
func (p *PromptLoader) defaultIssuePrompt(kind string) string {
	switch strings.ToLower(kind) {
	case "deployment":
		return defaultDeploymentPrompt
	case "pod":
		return defaultPodPrompt
	case "namespace":
		return defaultNamespacePrompt
	case "node":
		return defaultNodePrompt
	case "ingress":
		return defaultIngressPrompt
	case "pvc":
		return defaultPVCPrompt
	default:
		return ""
	}
}

// InstallDefaultPrompts installs default prompt files on first run
func InstallDefaultPrompts() error {
	home, _ := os.UserHomeDir()
	promptDir := filepath.Join(home, ".steered", "skills", "resources")
	if err := os.MkdirAll(promptDir, 0755); err != nil {
		return err
	}

	files := map[string]string{
		"base.md":                 defaultBasePrompt,
		"resources/deployment.md": defaultDeploymentPrompt,
		"resources/pod.md":        defaultPodPrompt,
		"resources/namespace.md":  defaultNamespacePrompt,
		"resources/node.md":       defaultNodePrompt,
		"resources/ingress.md":    defaultIngressPrompt,
		"resources/pvc.md":        defaultPVCPrompt,
	}

	for name, content := range files {
		path := filepath.Join(home, ".steered", "skills", name)
		// never overwrite user edits
		if _, err := os.Stat(path); os.IsNotExist(err) {
			if err := os.WriteFile(path, []byte(content), 0644); err != nil {
				return err
			}
		}
	}
	return nil
}
