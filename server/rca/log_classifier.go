package rca

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/onkar717/visual-eyes/server/models"
)

// logPattern classifies a log line into a named error category.
type logPattern struct {
	name string
	re   *regexp.Regexp
}

var logPatterns = []logPattern{
	{"fatal/critical/panic", regexp.MustCompile(`(?i)(fatal|critical|panic:)`)},
	{"error/exception/traceback", regexp.MustCompile(`(?i)(exception|traceback|error:|ERROR\s)`)},
	{"oom/out-of-memory", regexp.MustCompile(`(?i)(OOMKilled|OutOfMemory|out of memory|Killed)`)},
	{"connection/timeout", regexp.MustCompile(`(?i)(connection refused|connection reset|i\/o timeout|dial tcp|no such host|EOF)`)},
	{"auth/permission", regexp.MustCompile(`(?i)(401|403|Unauthorized|Forbidden|permission denied|access denied)`)},
	{"crashloop", regexp.MustCompile(`(?i)(CrashLoopBackOff|back-off restarting)`)},
	{"disk-full", regexp.MustCompile(`(?i)(no space left|disk quota|ENOSPC)`)},
	{"segfault/signal", regexp.MustCompile(`(?i)(segfault|SIGSEGV|signal: killed|core dumped)`)},
}

// stackTraceRe detects the start of a stack trace for Python, Go, Java, or panic output.
var stackTraceRe = regexp.MustCompile(`(?i)(goroutine \d+|Traceback \(most recent call|at [a-zA-Z0-9./$_]+\(|panic:|Exception in thread)`)

// ClassifiedLogs is the result of pre-LLM log analysis.
type ClassifiedLogs struct {
	// CategoryCounts maps category name → number of matched lines.
	CategoryCounts map[string]int
	// TopErrors holds at most 15 representative error lines with their category.
	TopErrors []ClassifiedLine
	// StackTraces holds extracted multi-line stack trace blocks (up to 3).
	StackTraces []string
	// Summary is a pre-built text block ready to prepend to the LLM log stage.
	Summary string
}

// ClassifiedLine is a single log line tagged with its error category.
type ClassifiedLine struct {
	Category string
	Line     string
}

// ClassifyLogs runs deterministic pattern matching on pod log lines before
// the LLM log-analysis stage. This surfaces error categories and stack traces
// cheaply so the LLM focuses on interpretation rather than pattern detection.
func ClassifyLogs(logs []models.PodLog, prevLogs []models.PodLog) ClassifiedLogs {
	all := append(logs, prevLogs...)
	if len(all) == 0 {
		return ClassifiedLogs{}
	}

	counts := make(map[string]int, len(logPatterns))
	var topErrors []ClassifiedLine
	var stackBuf []string
	var stackTraces []string
	inStack := false

	for _, l := range all {
		line := l.Line

		// Stack trace accumulation.
		if stackTraceRe.MatchString(line) {
			inStack = true
			stackBuf = []string{line}
			continue
		}
		if inStack {
			// Continue collecting if line looks like a stack frame.
			if strings.HasPrefix(line, "\t") || strings.HasPrefix(line, "  ") ||
				strings.HasPrefix(line, "at ") || strings.HasPrefix(line, "goroutine") {
				stackBuf = append(stackBuf, line)
				if len(stackBuf) > 30 {
					inStack = false
					if len(stackTraces) < 3 {
						stackTraces = append(stackTraces, strings.Join(stackBuf, "\n"))
					}
					stackBuf = nil
				}
				continue
			}
			// Stack ended.
			if len(stackBuf) > 2 && len(stackTraces) < 3 {
				stackTraces = append(stackTraces, strings.Join(stackBuf, "\n"))
			}
			stackBuf = nil
			inStack = false
		}

		// Category matching.
		for _, p := range logPatterns {
			if p.re.MatchString(line) {
				counts[p.name]++
				if len(topErrors) < 15 {
					topErrors = append(topErrors, ClassifiedLine{Category: p.name, Line: truncLine(line, 200)})
				}
				break // only count first matching category per line
			}
		}
	}

	// Flush open stack.
	if inStack && len(stackBuf) > 2 && len(stackTraces) < 3 {
		stackTraces = append(stackTraces, strings.Join(stackBuf, "\n"))
	}

	summary := buildLogSummary(counts, topErrors, stackTraces, len(prevLogs) > 0)
	return ClassifiedLogs{
		CategoryCounts: counts,
		TopErrors:      topErrors,
		StackTraces:    stackTraces,
		Summary:        summary,
	}
}

func buildLogSummary(counts map[string]int, topErrors []ClassifiedLine, traces []string, hasPrev bool) string {
	if len(counts) == 0 && len(topErrors) == 0 {
		return "(no error patterns detected in logs)"
	}

	var sb strings.Builder
	sb.WriteString("=== PRE-CLASSIFIED LOG PATTERNS ===\n")

	if len(counts) > 0 {
		sb.WriteString("Error category frequencies:\n")
		for _, p := range logPatterns {
			if n, ok := counts[p.name]; ok && n > 0 {
				sb.WriteString(fmt.Sprintf("  %-30s %d occurrences\n", p.name, n))
			}
		}
		sb.WriteString("\n")
	}

	if len(topErrors) > 0 {
		sb.WriteString(fmt.Sprintf("Top error lines (up to 15 of %d matched):\n", len(topErrors)))
		for i, e := range topErrors {
			sb.WriteString(fmt.Sprintf("  [%d][%s] %s\n", i+1, e.Category, e.Line))
		}
		sb.WriteString("\n")
	}

	if len(traces) > 0 {
		sb.WriteString(fmt.Sprintf("Stack traces detected (%d):\n", len(traces)))
		for i, t := range traces {
			sb.WriteString(fmt.Sprintf("--- trace %d ---\n%s\n", i+1, truncLine(t, 500)))
		}
		sb.WriteString("\n")
	}

	if hasPrev {
		sb.WriteString("NOTE: Previous container logs included — focus on pre-crash errors for CrashLoopBackOff diagnosis.\n")
	}

	return sb.String()
}

func truncLine(s string, max int) string {
	if len(s) > max {
		return s[:max] + "…"
	}
	return s
}
