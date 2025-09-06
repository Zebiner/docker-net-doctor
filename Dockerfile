# Dockerfile - Multi-stage build for optimal image size
# This creates a minimal container that can run the diagnostic tool

# Stage 1: Build the binary
FROM golang:1.21-alpine AS builder

# Install build dependencies
# We need git for version information and ca-certificates for HTTPS
RUN apk add --no-cache git ca-certificates tzdata

# Set working directory
WORKDIR /build

# Copy go mod files first for better caching
# Docker caches layers, so we want dependency downloads cached
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary with all optimizations
# -ldflags "-w -s" strips debug info for smaller size
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-w -s -X main.Version=$(git describe --tags --always) \
    -X main.BuildTime=$(date -u '+%Y-%m-%d_%H:%M:%S') \
    -X main.GitCommit=$(git rev-parse --short HEAD)" \
    -o docker-net-doctor \
    cmd/docker-net-doctor/main.go

# Stage 2: Create minimal runtime image
FROM scratch

# Copy necessary files from builder
# ca-certificates for HTTPS, tzdata for timezones, passwd for user info
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=builder /etc/passwd /etc/passwd

# Copy the binary
COPY --from=builder /build/docker-net-doctor /docker-net-doctor

# Run as non-root user for security
USER nobody

# Set the entrypoint
ENTRYPOINT ["/docker-net-doctor"]

# Default command (can be overridden)
CMD ["diagnose"]
