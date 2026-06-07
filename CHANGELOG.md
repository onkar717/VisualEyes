# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- GitHub Actions CI workflow (Go build, lint, markdown, yaml, shell checks)
- GitHub Actions release workflow with cross-platform binary builds
- `make cross` target for cross-compiling all binaries to `dist/`
- INSTALLATION.md with detailed setup guide for all deployment modes
- Component READMEs for system agent, kubernetes agent, and veye CLI
- docs/images directory for screenshots and architecture diagrams

### Changed
- README overhauled with badges, ASCII architecture diagram, and comprehensive guide
- Makefile updated with version injection via ldflags and cross-compile target

---

## [1.1.0] - 2026-06-07

### Added
- `veye watch` — full Bubbletea interactive TUI dashboard
- `veye logs --follow` — live pod log tail
- `veye alerts` — active alerts table view
- `veye rca` — RCA detail view per incident
- `veye status` — live cluster and system health snapshot
- veye CLI scaffold with Cobra root and HTTP client

### Fixed
- MemoryStore extended to full storage interface
- WebSocket hijack issue resolved
- UI field name alignment with backend models

---

## [1.0.0] - 2026-05-16

### Added
- React UI overhaul — Alerts panel, RCA drawer, Log viewer, WebSocket live updates
- Prometheus `/metrics` registry and WebSocket real-time metric streaming
- Claude AI-powered RCA engine with autonomous safe-command execution
- Pod log collection pipeline and Kubernetes Events collection
- Alert engine with configurable rules, dedup, noise filter, and auto-RCA trigger
- PostgreSQL backend via GORM (replaced in-memory storage)
- Backend architecture restructure: slog structured logging, middleware stack, health endpoints, graceful shutdown
- System metrics agent: CPU, memory, disk, network, load average via `gopsutil`
- Kubernetes agent: kubelet summary API, 42 metrics per cycle, DaemonSet deployment
- Backend server: REST API on port 8080, in-memory and PostgreSQL storage
- React + MUI + Vite dashboard: System and Kubernetes views, dark/light themes
- Docker support for all components
- Kubernetes RBAC, ConfigMap, and DaemonSet deployment manifests
- Docker Compose for full-stack local deployment
- `configs/config.yaml` hierarchical configuration
- MIT License, CONTRIBUTING.md, SECURITY.md

---

[Unreleased]: https://github.com/onkar717/visual-eyes/compare/v1.1.0...HEAD
[1.1.0]: https://github.com/onkar717/visual-eyes/compare/v1.0.0...v1.1.0
[1.0.0]: https://github.com/onkar717/visual-eyes/releases/tag/v1.0.0
