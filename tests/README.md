# Tests

## Running Tests

```bash
# All Go tests
make test

# With race detector (use before submitting PRs)
make test-race

# Specific package
go test -v ./backend/alerts/...
go test -v ./backend/storage/...
go test -v ./cli/...
```

## Test Structure

Tests live alongside the code they test (Go convention):

```
backend/alerts/engine_test.go
backend/storage/memory_test.go
backend/storage/postgres_test.go
agents/system/metrics/cpu_test.go
cli/cmd/status_test.go
```

## Writing Tests

Follow standard Go testing conventions:

```go
func TestAlertEngine_Evaluate(t *testing.T) {
    engine := alerts.NewEngine(defaultRules())
    metrics := testMetrics(cpuPercent: 95.0)

    fired := engine.Evaluate(metrics)

    if len(fired) == 0 {
        t.Fatal("expected SEV2 CPU alert, got none")
    }
    if fired[0].Severity != "SEV2" {
        t.Errorf("expected SEV2, got %s", fired[0].Severity)
    }
}
```

Use table-driven tests for multiple cases:

```go
func TestAlertSeverity(t *testing.T) {
    cases := []struct {
        cpuPercent float64
        wantSev    string
    }{
        {95.0, "SEV1"},
        {85.0, "SEV2"},
        {70.0, "SEV3"},
        {50.0, ""},
    }
    for _, tc := range cases {
        t.Run(fmt.Sprintf("cpu=%.0f%%", tc.cpuPercent), func(t *testing.T) {
            // ...
        })
    }
}
```

## Integration Tests

Integration tests that require a running PostgreSQL or Kubernetes cluster are skipped by default:

```go
func TestPostgresStorage(t *testing.T) {
    dsn := os.Getenv("TEST_DATABASE_URL")
    if dsn == "" {
        t.Skip("TEST_DATABASE_URL not set")
    }
    // ...
}
```

Run them with:

```bash
TEST_DATABASE_URL=postgres://user:pass@localhost:5432/testdb go test ./backend/storage/...
```

## CI

Tests run automatically on every push and pull request via `.github/workflows/ci-test.yaml` with the `-race` flag enabled.
