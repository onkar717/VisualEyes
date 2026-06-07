# Dockerfile for VisualEyes System Agent
# Build context must be the repo root (docker build -f system-agent/Dockerfile .)

FROM golang:1.24-alpine AS builder

WORKDIR /app

RUN apk --no-cache add ca-certificates git

# Copy entire workspace so go.work + all modules resolve correctly
COPY . .

RUN go work sync && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags='-w -s -extldflags "-static"' \
    -a -installsuffix cgo \
    -o visual-eyes-system-agent ./system-agent

FROM alpine:3.21

RUN apk --no-cache add ca-certificates tzdata

RUN addgroup -g 1001 -S appgroup && \
    adduser -u 1001 -S appuser -G appgroup

WORKDIR /app

COPY --from=builder /app/visual-eyes-system-agent .
COPY --from=builder /app/configs ./configs

RUN chown -R appuser:appgroup /app

USER appuser

HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
    CMD test -f /proc/1/status || exit 1

ENTRYPOINT ["./visual-eyes-system-agent"]
