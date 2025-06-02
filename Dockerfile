# Multi-stage build for pump-fun-bot-go
FROM golang:1.21-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s -X main.Version=$(git describe --tags --always --dirty 2>/dev/null || echo 'docker')" \
    -o pump-fun-bot \
    cmd/bot/main.go

# Final stage
FROM alpine:latest

# Install runtime dependencies
RUN apk --no-cache add ca-certificates tzdata

# Create non-root user
RUN adduser -D -s /bin/sh pump-bot

# Set working directory
WORKDIR /app

# Copy binary from builder stage
COPY --from=builder /app/pump-fun-bot .

# Copy configuration files
COPY configs/ ./configs/

# Create necessary directories
RUN mkdir -p logs trades && \
    chown -R pump-bot:pump-bot /app

# Switch to non-root user
USER pump-bot

# Expose metrics port (if enabled)
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD pgrep pump-fun-bot || exit 1

# Default command
CMD ["./pump-fun-bot"]

# Labels
LABEL maintainer="your-email@example.com"
LABEL description="Pump Fun Trading Bot - Go Implementation"
LABEL version="1.0.0"