.PHONY: build build-agent build-server run-agent run-server test clean fmt lint deps help

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOFMT=$(GOCMD) fmt
GOLINT=golangci-lint

# Binary name
BINARY_NAME_AGENT=visual-eyes-agent
BINARY_NAME_SERVER=visual-eyes-server

# Main paths
AGENT_PATH=cmd/agent/main.go
SERVER_PATH=cmd/server/main.go

# Build all
build: build-agent build-server

# Build agent
build-agent:
	$(GOBUILD) -o bin/$(BINARY_NAME_AGENT) $(AGENT_PATH)

# Build server
build-server:
	$(GOBUILD) -o bin/$(BINARY_NAME_SERVER) $(SERVER_PATH)

# Run agent locally
run-agent:
	./bin/$(BINARY_NAME_AGENT)

# Run agent with host metrics only
run-agent-host:
	VISUAL_EYES_AGENT_DISABLE_KUBE_METRICS=true \
	VISUAL_EYES_AGENT_DISABLE_HOST_METRICS=false \
	./bin/$(BINARY_NAME_AGENT)

# Run server
run-server:
	./bin/$(BINARY_NAME_SERVER)

# Run tests
test:
	$(GOTEST) -v ./...

# Clean binaries
clean:
	$(GOCLEAN)
	rm -f bin/$(BINARY_NAME_AGENT)
	rm -f bin/$(BINARY_NAME_SERVER)

# Create necessary directories
init:
	mkdir -p bin
	mkdir -p configs

fmt:
	$(GOFMT) ./...

lint:
	$(GOLINT) run

deps:
	$(GOCMD) mod download
	$(GOCMD) mod tidy

help:
	@echo "Make targets:"
	@echo "  build        - Build both agent and server"
	@echo "  build-agent  - Build agent binary"
	@echo "  build-server - Build server binary"
	@echo "  run-agent    - Run agent locally (default config)"
	@echo "  run-agent-host - Run agent with host metrics only"
	@echo "  run-server   - Run server locally"
	@echo "  test         - Run tests"
	@echo "  clean        - Clean build artifacts"
	@echo "  init         - Create necessary directories"
	@echo "  fmt        - Format code"
	@echo "  lint       - Run linter"
	@echo "  deps       - Download and tidy dependencies"
	@echo ""
	@echo "Note: Kubernetes agent runs via DaemonSet using Docker image"
	@echo "      This Makefile is for local development only"