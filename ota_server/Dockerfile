# Multi-stage build for Go OTA Server
FROM --platform=$BUILDPLATFORM golang:1.22-alpine AS builder

# Build arguments for cross-compilation
ARG TARGETOS
ARG TARGETARCH

# Set working directory
WORKDIR /app

# Install git and ca-certificates (needed for go mod download)
RUN apk add --no-cache git ca-certificates

# Copy go mod files first for better caching
COPY go.* ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application with explicit architecture
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} go build -a -installsuffix cgo -o ota-server .

# Final stage - minimal image
FROM alpine:latest

# Install ca-certificates and wget for health checks
RUN apk --no-cache add ca-certificates wget

# Create non-root user
RUN addgroup -g 1001 -S appgroup && \
    adduser -u 1001 -S appuser -G appgroup

# Set working directory
WORKDIR /app

# Create firmware directory with proper permissions
RUN mkdir -p /app/firmware && \
    chown -R appuser:appgroup /app

# Copy binary from builder stage
COPY --from=builder --chown=appuser:appgroup /app/ota-server .

# Switch to non-root user
USER appuser

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/version || exit 1

# Set volume for firmware persistence
VOLUME ["/app/firmware"]

# Run the application
CMD ["./ota-server"]