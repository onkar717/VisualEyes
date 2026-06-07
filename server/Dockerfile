# Dockerfile for VisualEyes Backend Server
# Build context must be the repo root (docker build -f server/Dockerfile .)

FROM golang:1.24-alpine AS builder

WORKDIR /app

RUN apk --no-cache add ca-certificates git

# Copy entire workspace so go.work + all modules resolve correctly
COPY . .

RUN go work sync && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags='-w -s -extldflags "-static"' \
    -a -installsuffix cgo \
    -o visual-eyes-server ./server

FROM alpine:3.21

RUN apk --no-cache add ca-certificates tzdata wget

RUN addgroup -g 1001 -S appgroup && \
    adduser -u 1001 -S appuser -G appgroup

WORKDIR /app

COPY --from=builder /app/visual-eyes-server .
COPY --from=builder /app/configs ./configs

RUN chown -R appuser:appgroup /app

USER appuser

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/ping || exit 1

ENTRYPOINT ["./visual-eyes-server"]
