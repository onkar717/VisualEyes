package rca

import (
	"embed"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed runbooks/*.yaml
var runbookFS embed.FS

// RunbookStep is one remediation action in a runbook.
type RunbookStep struct {
	Step        int    `yaml:"step"`
	Description string `yaml:"description"`
	Command     string `yaml:"command_template"`
	IsAutoSafe  bool   `yaml:"is_auto_safe"`
}

// Runbook is a structured remediation playbook for a specific incident category.
type Runbook struct {
	Name        string        `yaml:"name"`
	Description string        `yaml:"description"`
	Triggers    []string      `yaml:"triggers"`
	Categories  []string      `yaml:"categories"`
	Remediation []RunbookStep `yaml:"remediation"`
}

var loadedRunbooks []*Runbook

func init() {
	entries, _ := runbookFS.ReadDir("runbooks")
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		data, err := runbookFS.ReadFile("runbooks/" + e.Name())
		if err != nil {
			continue
		}
		var rb Runbook
		if err := yaml.Unmarshal(data, &rb); err != nil {
			continue
		}
		loadedRunbooks = append(loadedRunbooks, &rb)
	}
}

// SelectRunbook returns the best matching runbook for the given category string.
// Returns nil when no match found.
func SelectRunbook(category string) *Runbook {
	cat := strings.ToLower(strings.TrimSpace(category))
	for _, rb := range loadedRunbooks {
		for _, c := range rb.Categories {
			if strings.EqualFold(c, cat) {
				return rb
			}
		}
	}
	// Fuzzy fallback: partial name match.
	for _, rb := range loadedRunbooks {
		if strings.Contains(cat, rb.Name) || strings.Contains(rb.Name, cat) {
			return rb
		}
	}
	return nil
}

// RunbookSummary returns a compact text summary of a runbook for inclusion in LLM prompts.
func RunbookSummary(rb *Runbook) string {
	if rb == nil {
		return "(no matching runbook)"
	}
	var sb strings.Builder
	sb.WriteString("Runbook: " + rb.Name + " — " + rb.Description + "\n")
	for _, step := range rb.Remediation {
		safe := "manual"
		if step.IsAutoSafe {
			safe = "auto-safe"
		}
		sb.WriteString("  " + step.Description + " [" + safe + "]\n")
		sb.WriteString("    cmd: " + step.Command + "\n")
	}
	return sb.String()
}
