# Dockerfile for VisualEyes Kubernetes Agent
# Used by the Kubernetes DaemonSet deployment
# Build context must be the repo root

FROM golang:1.24-alpine AS builder

WORKDIR /app

RUN apk --no-cache add ca-certificates git

# Copy entire workspace so go.work + all modules resolve correctly
COPY . .

RUN go work sync && \
    CGO_ENABLED=0 GOOS=linux go build \
    -ldflags='-w -s -extldflags "-static"' \
    -o visual-eyes-kube-agent ./k8s-agent

FROM alpine:3.21

RUN apk --no-cache add ca-certificates

WORKDIR /root/

COPY --from=builder /app/visual-eyes-kube-agent /bin/visual-eyes-kube-agent
COPY --from=builder /app/configs /configs

ENTRYPOINT ["/bin/visual-eyes-kube-agent"]
