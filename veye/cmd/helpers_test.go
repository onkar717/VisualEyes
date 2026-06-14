package cmd

import (
	"strings"
	"testing"
)

// ── truncStr (status.go) ──────────────────────────────────────────────────────

func TestTruncStr_Short(t *testing.T) {
	got := truncStr("hello", 20)
	if got != "hello" {
		t.Errorf("want %q, got %q", "hello", got)
	}
}

func TestTruncStr_Exact(t *testing.T) {
	s := strings.Repeat("x", 10)
	got := truncStr(s, 10)
	if got != s {
		t.Errorf("want unchanged at exact limit, got %q", got)
	}
}

func TestTruncStr_Over_AddsEllipsis(t *testing.T) {
	got := truncStr("hello world long string", 10)
	if !strings.HasSuffix(got, "…") {
		t.Errorf("want ellipsis suffix, got %q", got)
	}
	if len([]rune(got)) > 10 {
		t.Errorf("want ≤10 runes, got %d", len([]rune(got)))
	}
}

func TestTruncStr_Empty(t *testing.T) {
	got := truncStr("", 10)
	if got != "" {
		t.Errorf("want empty, got %q", got)
	}
}

// ── wordWrap (show.go) ────────────────────────────────────────────────────────

func TestWordWrap_ShortString(t *testing.T) {
	got := wordWrap("short", 40)
	if got != "short" {
		t.Errorf("want %q, got %q", "short", got)
	}
}

func TestWordWrap_LongString_WrapsAtWidth(t *testing.T) {
	input := "this is a somewhat long string that should wrap at some point"
	got := wordWrap(input, 20)
	lines := strings.Split(got, "\n")
	if len(lines) < 2 {
		t.Error("expected wrapped output to have multiple lines")
	}
	// No single segment (before indent prefix) should exceed width.
	for _, line := range lines {
		trimmed := strings.TrimPrefix(line, "  ")
		if len(trimmed) > 20 {
			t.Errorf("line %q exceeds width 20", trimmed)
		}
	}
}

func TestWordWrap_Empty(t *testing.T) {
	got := wordWrap("", 40)
	if got != "" {
		t.Errorf("want empty, got %q", got)
	}
}

// ── formatMTTR (incidents.go) ─────────────────────────────────────────────────

func TestFormatMTTR_Seconds(t *testing.T) {
	got := formatMTTR(45)
	if got != "45s" {
		t.Errorf("want 45s, got %q", got)
	}
}

func TestFormatMTTR_Minutes(t *testing.T) {
	got := formatMTTR(90) // 1.5 minutes
	if !strings.Contains(got, "m") {
		t.Errorf("want minutes format, got %q", got)
	}
}

func TestFormatMTTR_Hours(t *testing.T) {
	got := formatMTTR(7200) // 2 hours
	if !strings.Contains(got, "h") {
		t.Errorf("want hours format, got %q", got)
	}
}

func TestFormatMTTR_Zero(t *testing.T) {
	got := formatMTTR(0)
	if got != "0s" {
		t.Errorf("want 0s, got %q", got)
	}
}

// ── formatPercent (scan.go) ───────────────────────────────────────────────────

func TestFormatPercent_Zero(t *testing.T) {
	got := formatPercent(0)
	// 0 returns "n/a" styled
	if !strings.Contains(got, "n/a") {
		t.Errorf("want n/a for 0%%, got %q", got)
	}
}

func TestFormatPercent_Normal(t *testing.T) {
	got := formatPercent(50.0)
	if !strings.Contains(got, "50.0%") {
		t.Errorf("want 50.0%% in output, got %q", got)
	}
}

func TestFormatPercent_Critical(t *testing.T) {
	got := formatPercent(95.0)
	if !strings.Contains(got, "95.0%") {
		t.Errorf("want 95.0%% in output, got %q", got)
	}
}

// ── sevStyle (incidents.go) ───────────────────────────────────────────────────

func TestSevStyle_AllSeverities(t *testing.T) {
	cases := []string{"SEV1", "SEV2", "SEV3", "SEV4", "UNKNOWN"}
	for _, sev := range cases {
		style := sevStyle(sev)
		// Verify it renders without panic and returns a non-empty string.
		rendered := style.Render(sev)
		if rendered == "" {
			t.Errorf("sevStyle(%q).Render returned empty string", sev)
		}
	}
}
