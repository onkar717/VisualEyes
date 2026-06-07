package alerts

import (
	"fmt"
	"strings"

	"github.com/onkar717/visual-eyes/backend/config"
	"github.com/onkar717/visual-eyes/backend/models"
)

// Operator defines how a metric value is compared to a threshold.
type Operator string

const (
	OpGT  Operator = "gt"
	OpLT  Operator = "lt"
	OpGTE Operator = "gte"
	OpLTE Operator = "lte"
)

// Rule describes one anomaly condition the engine evaluates.
type Rule struct {
	Name       string
	MetricName string
	Threshold  float64
	Op         Operator
	Severity   models.AlertSeverity
	TagFilter  map[string]string
}

// Evaluate returns true if the metric value violates this rule's threshold.
func (r Rule) Evaluate(value float64) bool {
	switch r.Op {
	case OpGT:
		return value > r.Threshold
	case OpLT:
		return value < r.Threshold
	case OpGTE:
		return value >= r.Threshold
	case OpLTE:
		return value <= r.Threshold
	}
	return false
}

// Message returns a human-readable description of the violation.
func (r Rule) Message(resourceID string, value float64) string {
	return fmt.Sprintf("%s: %s=%.2f %s threshold %.2f (resource: %s)",
		r.Severity, r.MetricName, value, r.Op, r.Threshold, resourceID)
}

// DedupeKey produces a stable string that uniquely identifies an active alert
// for this rule and resource. Used to prevent duplicate alert records.
func (r Rule) DedupeKey(resourceID, namespace string) string {
	return fmt.Sprintf("%s|%s|%s", r.Name, resourceID, namespace)
}

// TagMatches returns true if a metric's tags contain all of this rule's tag filters.
func (r Rule) TagMatches(metricTags map[string]string) bool {
	for k, v := range r.TagFilter {
		if metricTags[k] != v {
			return false
		}
	}
	return true
}

// ResourceID extracts a meaningful resource identifier from metric tags.
func ResourceID(tags map[string]string) string {
	for _, key := range []string{"pod", "node", "container", "hostname"} {
		if v := tags[key]; v != "" {
			return v
		}
	}
	return "unknown"
}

// Namespace extracts the namespace tag or returns "default".
func Namespace(tags map[string]string) string {
	if ns := tags["namespace"]; ns != "" {
		return ns
	}
	return "default"
}

// FromConfig converts the slice of config.AlertRule into Rule structs.
// Unknown operators default to "gt". Unknown severities default to "warning".
func FromConfig(cfgRules []config.AlertRule) []Rule {
	rules := make([]Rule, 0, len(cfgRules))
	for _, r := range cfgRules {
		rules = append(rules, Rule{
			Name:       r.Name,
			MetricName: r.MetricName,
			Threshold:  r.Threshold,
			Op:         parseOperator(r.Operator),
			Severity:   parseSeverity(r.Severity),
			TagFilter:  r.TagFilter,
		})
	}
	return rules
}

func parseOperator(s string) Operator {
	switch strings.ToLower(s) {
	case "lt":
		return OpLT
	case "gte":
		return OpGTE
	case "lte":
		return OpLTE
	default:
		return OpGT
	}
}

func parseSeverity(s string) models.AlertSeverity {
	switch strings.ToLower(s) {
	case "critical":
		return models.SeverityCritical
	case "info":
		return models.SeverityInfo
	default:
		return models.SeverityWarning
	}
}
