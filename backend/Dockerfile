# Dockerfile for VisualEyes Backend Server
# Multi-stage build for optimized production image

FROM golang:1.24-alpine AS builder

# Set working directory
WORKDIR /app

# Install build dependencies
RUN apk --no-cache add ca-certificates git

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the backend server binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags='-w -s -extldflags "-static"' \
    -a -installsuffix cgo \
    -o visual-eyes-server ./backend

# Final stage - minimal runtime image
FROM alpine:3.21

# Install runtime dependencies
RUN apk --no-cache add ca-certificates tzdata wget

# Create non-root user for security
RUN addgroup -g 1001 -S appgroup && \
    adduser -u 1001 -S appuser -G appgroup

# Set working directory
WORKDIR /app

# Copy binary from builder stage
COPY --from=builder /app/visual-eyes-server .
COPY --from=builder /app/configs ./configs

# Change ownership to non-root user
RUN chown -R appuser:appgroup /app

# Switch to non-root user
USER appuser

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/ping || exit 1

# Environment variables will be set by docker-compose or at runtime
# Default values: GIN_MODE=release, PORT=8080

# Run the binary
ENTRYPOINT ["./visual-eyes-server"]
