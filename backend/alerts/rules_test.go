package alerts

import (
	"strings"
	"testing"

	"github.com/onkar717/visual-eyes/backend/models"
)

func TestRule_Evaluate(t *testing.T) {
	cases := []struct {
		op        Operator
		threshold float64
		value     float64
		want      bool
	}{
		{OpGT, 80, 85, true},
		{OpGT, 80, 80, false},
		{OpGT, 80, 75, false},
		{OpLT, 20, 10, true},
		{OpLT, 20, 20, false},
		{OpLT, 20, 25, false},
		{OpGTE, 80, 80, true},
		{OpGTE, 80, 81, true},
		{OpGTE, 80, 79, false},
		{OpLTE, 20, 20, true},
		{OpLTE, 20, 19, true},
		{OpLTE, 20, 21, false},
		{"unknown", 50, 60, false},
	}
	for _, tc := range cases {
		r := Rule{Op: tc.op, Threshold: tc.threshold}
		got := r.Evaluate(tc.value)
		if got != tc.want {
			t.Errorf("op=%s threshold=%.0f value=%.0f: got %v want %v",
				tc.op, tc.threshold, tc.value, got, tc.want)
		}
	}
}

func TestRule_Message(t *testing.T) {
	r := Rule{
		Name:       "high-cpu",
		MetricName: "cpu.usage_percent",
		Threshold:  85,
		Op:         OpGT,
		Severity:   models.SeverityWarning,
	}
	msg := r.Message("node-1", 92.5)
	for _, want := range []string{"cpu.usage_percent", "92.50", "85.00", "node-1"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message %q missing %q", msg, want)
		}
	}
}

func TestRule_DedupeKey(t *testing.T) {
	r := Rule{Name: "high-cpu"}
	key := r.DedupeKey("node-1", "kube-system")
	if key != "high-cpu|node-1|kube-system" {
		t.Errorf("unexpected dedupe key: %q", key)
	}
	if r.DedupeKey("node-1", "kube-system") != key {
		t.Error("dedupe key is not deterministic")
	}
}

func TestRule_TagMatches(t *testing.T) {
	r := Rule{TagFilter: map[string]string{"env": "prod", "region": "us-east-1"}}

	if !r.TagMatches(map[string]string{"env": "prod", "region": "us-east-1", "extra": "ok"}) {
		t.Error("expected match with superset tags")
	}
	if r.TagMatches(map[string]string{"env": "prod"}) {
		t.Error("expected no match — missing region tag")
	}
	if r.TagMatches(map[string]string{"env": "staging", "region": "us-east-1"}) {
		t.Error("expected no match — wrong env value")
	}

	empty := Rule{}
	if !empty.TagMatches(map[string]string{"any": "thing"}) {
		t.Error("empty tag filter should match any tags")
	}
}

func TestResourceID(t *testing.T) {
	cases := []struct {
		tags map[string]string
		want string
	}{
		{map[string]string{"pod": "web-abc"}, "web-abc"},
		{map[string]string{"node": "node-1"}, "node-1"},
		{map[string]string{"container": "nginx"}, "nginx"},
		{map[string]string{"hostname": "host-1"}, "host-1"},
		{map[string]string{"pod": "web", "node": "node-1"}, "web"},
		{map[string]string{"other": "val"}, "unknown"},
		{map[string]string{}, "unknown"},
	}
	for _, tc := range cases {
		got := ResourceID(tc.tags)
		if got != tc.want {
			t.Errorf("ResourceID(%v) = %q, want %q", tc.tags, got, tc.want)
		}
	}
}

func TestNamespace(t *testing.T) {
	if got := Namespace(map[string]string{"namespace": "kube-system"}); got != "kube-system" {
		t.Errorf("expected kube-system, got %q", got)
	}
	if got := Namespace(map[string]string{}); got != "default" {
		t.Errorf("expected default, got %q", got)
	}
}

func TestParseOperator(t *testing.T) {
	cases := []struct {
		in   string
		want Operator
	}{
		{"gt", OpGT},
		{"GT", OpGT},
		{"lt", OpLT},
		{"gte", OpGTE},
		{"lte", OpLTE},
		{"unknown", OpGT},
		{"", OpGT},
	}
	for _, tc := range cases {
		got := parseOperator(tc.in)
		if got != tc.want {
			t.Errorf("parseOperator(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestParseSeverity(t *testing.T) {
	cases := []struct {
		in   string
		want models.AlertSeverity
	}{
		{"critical", models.SeverityCritical},
		{"CRITICAL", models.SeverityCritical},
		{"info", models.SeverityInfo},
		{"warning", models.SeverityWarning},
		{"unknown", models.SeverityWarning},
		{"", models.SeverityWarning},
	}
	for _, tc := range cases {
		got := parseSeverity(tc.in)
		if got != tc.want {
			t.Errorf("parseSeverity(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}
