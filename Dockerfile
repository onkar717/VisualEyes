# Dockerfile for VisualEyes Kubernetes Agent
# This image is used by the Kubernetes DaemonSet deployment
# For local development, use 'make build' and 'make run-agent'

FROM golang:1.23-alpine AS builder

WORKDIR /app
COPY . .

RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o visual-eyes ./cmd/agent

FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /root/
COPY --from=builder /app/visual-eyes /bin/visual-eyes
COPY --from=builder /app/configs /configs

ENV VISUAL_EYES_CONFIG=/configs/config.yaml

ENTRYPOINT ["/bin/visual-eyes"] 