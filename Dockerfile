# Dockerfile for VisualEyes Kubernetes Agent
# This image is used by the Kubernetes DaemonSet deployment
# For local development, use 'make build' and 'make run-kube-agent'

FROM golang:1.23-alpine AS builder

WORKDIR /app
COPY . .

RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o visual-eyes-kube-agent ./agents/kubernetes

FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /root/
COPY --from=builder /app/visual-eyes-kube-agent /bin/visual-eyes-kube-agent
COPY --from=builder /app/configs /configs

# Environment variables will be set by Kubernetes deployment
# Default config path: /configs/config.yaml

ENTRYPOINT ["/bin/visual-eyes-kube-agent"] 