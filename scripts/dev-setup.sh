#!/bin/bash
# One-shot local development environment setup for VisualEyes.
# Run once after cloning: bash scripts/dev-setup.sh

set -euo pipefail

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

info()  { echo -e "${GREEN}[setup]${NC} $*"; }
warn()  { echo -e "${YELLOW}[warn]${NC}  $*"; }
error() { echo -e "${RED}[error]${NC} $*"; exit 1; }

# ── Check required tools ──────────────────────────────────────────────────────

check_tool() {
  if ! command -v "$1" &>/dev/null; then
    error "$1 is required but not installed. See: $2"
  fi
  info "$1 found: $(command -v "$1")"
}

info "Checking required tools..."
check_tool go       "https://golang.org/dl"
check_tool node     "https://nodejs.org"
check_tool npm      "https://nodejs.org"
check_tool docker   "https://docs.docker.com/get-docker"
check_tool git      "https://git-scm.com"

# ── Go version check ──────────────────────────────────────────────────────────

REQUIRED_GO="1.24"
CURRENT_GO=$(go version | awk '{print $3}' | sed 's/go//')
if [ "$(printf '%s\n' "$REQUIRED_GO" "$CURRENT_GO" | sort -V | head -n1)" != "$REQUIRED_GO" ]; then
  error "Go $REQUIRED_GO+ required, found $CURRENT_GO"
fi

# ── Go dependencies ───────────────────────────────────────────────────────────

info "Downloading Go dependencies..."
go mod download
go mod tidy

# ── Build all binaries ────────────────────────────────────────────────────────

info "Building all binaries..."
make build
info "Binaries in bin/: $(ls bin/)"

# ── Frontend ──────────────────────────────────────────────────────────────────

info "Installing frontend dependencies..."
(cd ui && npm ci)

# ── Environment file ──────────────────────────────────────────────────────────

if [ ! -f ".env" ]; then
  cp .env.example .env
  warn ".env created from .env.example"
  warn "Edit .env and set ANTHROPIC_API_KEY for AI RCA (optional)"
  warn "Set DATABASE_URL for PostgreSQL (optional defaults to in-memory)"
else
  info ".env already exists, skipping"
fi

# ── Optional tools ────────────────────────────────────────────────────────────

info "Checking optional tools..."

if command -v golangci-lint &>/dev/null; then
  info "golangci-lint found: $(golangci-lint version 2>&1 | head -1)"
else
  warn "golangci-lint not found install for 'make lint': https://golangci-lint.run/usage/install/"
fi

if command -v kubectl &>/dev/null; then
  info "kubectl found: $(kubectl version --client --short 2>/dev/null || kubectl version --client 2>/dev/null | head -1)"
else
  warn "kubectl not found needed for Kubernetes mode: https://kubernetes.io/docs/tasks/tools/"
fi

# ── Done ──────────────────────────────────────────────────────────────────────

echo ""
info "Setup complete. Quick start:"
echo "  ./bin/visual-eyes-server   # Backend on :8080"
echo "  ./bin/visual-eyes-agent    # System metrics push"
echo "  make run-ui                # React UI on :5173"
echo "  ./bin/veye watch           # Interactive TUI"
echo ""
info "Or: docker-compose up --build -d  (full stack)"
