.PHONY: build build-system-agent build-kube-agent build-server build-cli cross install install-cli run-system-agent run-kube-agent run-server run-ui install-ui build-ui build-docker build-docker-tag push-docker build-and-push build-docker-server run-docker-server push-docker-server build-docker-ui run-docker-ui push-docker-ui build-docker-system-agent run-docker-system-agent push-docker-system-agent docker-up docker-down docker-logs deploy-k8s undeploy-k8s status-k8s run-all test test-race clean fmt lint tidy deps help

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
BINARY_NAME_CLI=veye

# Version
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS=-ldflags "-X main.Version=$(VERSION) -s -w"
LDFLAGS_DEV=-ldflags "-X main.Version=$(VERSION)"

# Install dir
INSTALL_DIR ?= /usr/local/bin

# Main paths (multi-module layout)
SYSTEM_AGENT_PATH=./system-agent
KUBE_AGENT_PATH=./k8s-agent
SERVER_PATH=./server
CLI_PATH=./veye

# Build all
build: build-system-agent build-kube-agent build-server build-cli

# Build system agent
build-system-agent:
	$(GOBUILD) $(LDFLAGS) -o bin/$(BINARY_NAME_AGENT) $(SYSTEM_AGENT_PATH)

# Build kubernetes agent
build-kube-agent:
	$(GOBUILD) $(LDFLAGS) -o bin/visual-eyes-kube-agent $(KUBE_AGENT_PATH)

# Build server
build-server:
	$(GOBUILD) $(LDFLAGS) -o bin/$(BINARY_NAME_SERVER) $(SERVER_PATH)

# Build CLI
build-cli:
	$(GOBUILD) $(LDFLAGS) -o bin/$(BINARY_NAME_CLI) $(CLI_PATH)

# Cross-compile all binaries for release
cross:
	mkdir -p dist
	GOOS=linux   GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o dist/visual-eyes-linux-amd64   $(SERVER_PATH)
	GOOS=linux   GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o dist/visual-eyes-linux-arm64   $(SERVER_PATH)
	GOOS=darwin  GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o dist/visual-eyes-darwin-amd64  $(SERVER_PATH)
	GOOS=darwin  GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o dist/visual-eyes-darwin-arm64  $(SERVER_PATH)
	GOOS=windows GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o dist/visual-eyes-windows-amd64.exe $(SERVER_PATH)
	GOOS=linux   GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o dist/veye-linux-amd64          $(CLI_PATH)
	GOOS=linux   GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o dist/veye-linux-arm64          $(CLI_PATH)
	GOOS=darwin  GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o dist/veye-darwin-amd64         $(CLI_PATH)
	GOOS=darwin  GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o dist/veye-darwin-arm64         $(CLI_PATH)
	GOOS=windows GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o dist/veye-windows-amd64.exe    $(CLI_PATH)

# Run system agent locally
run-system-agent:
	./bin/$(BINARY_NAME_AGENT)

# Run system agent with host metrics only
run-system-agent-host:
	VISUAL_EYES_AGENT_DISABLE_KUBE_METRICS=true \
	VISUAL_EYES_AGENT_DISABLE_HOST_METRICS=false \
	./bin/$(BINARY_NAME_AGENT)

# Run kubernetes agent
run-kube-agent:
	./bin/visual-eyes-kube-agent

# Run server
run-server:
	./bin/$(BINARY_NAME_SERVER)

# Run UI (requires Node.js)
run-ui:
	cd ui && npm run dev

# Install UI dependencies
install-ui:
	cd ui && npm install

# Build UI for production
build-ui:
	cd ui && npm run build

# Build Docker image for Kubernetes agent
build-docker:
	docker build -t visual-eyes-kube-agent:latest .

# Build and tag Docker image with version
build-docker-tag:
	docker build -t visual-eyes-kube-agent:latest -t visual-eyes-kube-agent:$(shell date +%Y%m%d-%H%M%S) .

# Push Docker image to Docker Hub
push-docker:
	docker tag visual-eyes-kube-agent:latest onkar4545/visual-eyes-kube-agent:latest
	docker push onkar4545/visual-eyes-kube-agent:latest

# Build and push Docker image in one command
build-and-push:
	make build-docker
	make push-docker

# Build Docker image for backend server
build-docker-server:
	docker build -f server/Dockerfile -t visual-eyes-server:latest .

# Run backend server in Docker
run-docker-server:
	docker run --rm -p 8080:8080 --name visual-eyes-server visual-eyes-server:latest

# Push backend server to Docker Hub
push-docker-server:
	docker tag visual-eyes-server:latest onkar4545/visual-eyes-server:latest
	docker push onkar4545/visual-eyes-server:latest

# Build Docker image for UI
build-docker-ui:
	docker build -f ui/Dockerfile -t visual-eyes-ui:latest ./ui

# Run UI in Docker
run-docker-ui:
	docker run --rm -p 3000:3000 --name visual-eyes-ui visual-eyes-ui:latest

# Push UI to Docker Hub
push-docker-ui:
	docker tag visual-eyes-ui:latest onkar4545/visual-eyes-ui:latest
	docker push onkar4545/visual-eyes-ui:latest

# Build Docker image for system agent
build-docker-system-agent:
	docker build -f system-agent/Dockerfile -t visual-eyes-system-agent:latest .

# Run system agent in Docker
run-docker-system-agent:
	docker run --rm --name visual-eyes-system-agent visual-eyes-system-agent:latest

# Push system agent to Docker Hub
push-docker-system-agent:
	docker tag visual-eyes-system-agent:latest onkar4545/visual-eyes-system-agent:latest
	docker push onkar4545/visual-eyes-system-agent:latest

# Docker Compose commands
docker-up:
	docker-compose up --build -d

docker-down:
	docker-compose down

docker-logs:
	docker-compose logs -f

# Build all Docker images
build-all-docker: build-docker-server build-docker-ui build-docker-system-agent build-docker

# Push all Docker images
push-all-docker: push-docker-server push-docker-ui push-docker-system-agent push-docker

# Deploy to Kubernetes
deploy-k8s:
	kubectl apply -f deployments/kubernetes/

# Remove from Kubernetes
undeploy-k8s:
	kubectl delete -f deployments/kubernetes/

# Check Kubernetes deployment status
status-k8s:
	kubectl get pods -n kube-system -l app=visual-eyes-k8s-agent

# Run all components (server + system agent + UI)
run-all: run-server run-system-agent run-ui

# Run tests across all modules
test:
	$(GOTEST) -v ./server/... ./veye/... ./system-agent/... ./k8s-agent/...

# Run tests with race detector
test-race:
	$(GOTEST) -v -race -count=1 ./server/... ./veye/... ./system-agent/... ./k8s-agent/...

# Install veye CLI to INSTALL_DIR (default: /usr/local/bin)
install-cli: build-cli
	@if [ -w "$(INSTALL_DIR)" ]; then \
		cp bin/$(BINARY_NAME_CLI) $(INSTALL_DIR)/$(BINARY_NAME_CLI); \
	else \
		sudo cp bin/$(BINARY_NAME_CLI) $(INSTALL_DIR)/$(BINARY_NAME_CLI); \
	fi
	@echo "$(BINARY_NAME_CLI) installed to $(INSTALL_DIR)/$(BINARY_NAME_CLI)"

# Install server binary to INSTALL_DIR
install: build-server
	@if [ -w "$(INSTALL_DIR)" ]; then \
		cp bin/$(BINARY_NAME_SERVER) $(INSTALL_DIR)/$(BINARY_NAME_SERVER); \
	else \
		sudo cp bin/$(BINARY_NAME_SERVER) $(INSTALL_DIR)/$(BINARY_NAME_SERVER); \
	fi
	@echo "$(BINARY_NAME_SERVER) installed to $(INSTALL_DIR)/$(BINARY_NAME_SERVER)"

# Clean binaries
clean:
	$(GOCLEAN)
	rm -f bin/$(BINARY_NAME_AGENT)
	rm -f bin/$(BINARY_NAME_SERVER)
	rm -f bin/$(BINARY_NAME_CLI)
	rm -rf dist/

# Create necessary directories
init:
	mkdir -p bin
	mkdir -p configs

## ── AI-SRE Python service ────────────────────────────────────────────────────

AI_SRE_DIR=./ai-sre

ai-sre-install:
	pip install -r $(AI_SRE_DIR)/requirements.txt

ai-sre-dev:
	pip install -r $(AI_SRE_DIR)/requirements.txt -r $(AI_SRE_DIR)/requirements-dev.txt

ai-sre-lint:
	ruff check $(AI_SRE_DIR)/
	mypy $(AI_SRE_DIR)/ --ignore-missing-imports --no-error-summary || true

ai-sre-serve:
	cd $(AI_SRE_DIR) && uvicorn main:app --reload --host 0.0.0.0 --port 8001

ai-sre-scan:
	cd $(AI_SRE_DIR) && python -m cli scan

ai-sre-build:
	docker build -t visual-eyes-ai-sre:latest $(AI_SRE_DIR)

## ─────────────────────────────────────────────────────────────────────────────

fmt:
	$(GOFMT) ./...

lint:
	$(GOLINT) run

tidy:
	$(GOCMD) mod tidy

deps:
	$(GOCMD) mod download
	$(GOCMD) mod tidy

help:
	@echo "Make targets:"
	@echo "  build                - Build all components (system agent, kube agent, server)"
	@echo "  build-system-agent   - Build system agent binary"
	@echo "  build-kube-agent     - Build Kubernetes agent binary"
	@echo "  build-server         - Build backend server binary"
	@echo "  run-system-agent     - Run system agent locally"
	@echo "  run-system-agent-host - Run system agent with host metrics only"
	@echo "  run-kube-agent       - Run Kubernetes agent locally"
	@echo "  run-server           - Run backend server locally"
	@echo "  run-ui               - Run UI (requires Node.js)"
	@echo "  run-all              - Run server + system agent + UI together"
	@echo "  install-ui           - Install UI dependencies"
	@echo "  build-ui             - Build UI for production"
	@echo "  build-docker         - Build Docker image for Kubernetes agent"
	@echo "  build-docker-tag     - Build Docker image with timestamp tag"
	@echo "  push-docker          - Push Docker image to Docker Hub"
	@echo "  build-and-push       - Build and push Docker image in one command"
	@echo "  deploy-k8s           - Deploy to Kubernetes cluster"
	@echo "  undeploy-k8s         - Remove from Kubernetes cluster"
	@echo "  status-k8s           - Check Kubernetes deployment status"
	@echo "  test                 - Run tests"
	@echo "  clean                - Clean build artifacts"
	@echo "  fmt                  - Format code"
	@echo "  lint                 - Run linter"
	@echo "  deps                 - Download and tidy dependencies"
	@echo ""
	@echo "AI-SRE (Python CrewAI service):"
	@echo "  ai-sre-install       - Install Python runtime dependencies"
	@echo "  ai-sre-dev           - Install runtime + dev dependencies"
	@echo "  ai-sre-lint          - Run ruff + mypy over ai-sre/"
	@echo "  ai-sre-serve         - Start FastAPI service (hot-reload, port 8001)"
	@echo "  ai-sre-scan          - Run standalone AI-SRE scan (no Go server needed)"
	@echo "  ai-sre-build         - Build ai-sre Docker image"
	@echo ""
	@echo "Quick Start:"
	@echo "  1. make build        - Build all components"
	@echo "  2. make install-ui   - Install UI dependencies"
	@echo "  3. make run-all      - Start everything together"
	@echo "      This Makefile is for local development only"