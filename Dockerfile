# Build stage
FROM golang:1.23-alpine AS builder

WORKDIR /app

# Allow passing target OS/ARCH from buildx for proper cross-compilation
ARG TARGETOS=linux
ARG TARGETARCH=amd64
ARG TARGETVARIANT=""
ENV GO111MODULE=on

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Update dependencies to fix missing go.sum entry for golang.org/x/sync/semaphore
RUN go get github.com/jackc/puddle/v2@v2.2.1

# Build the application for the target platform
# Set GOARM only when building for arm (32-bit). For arm64, GOARCH=arm64 is sufficient.
RUN \
    if [ "$TARGETARCH" = "arm" ] && [ -n "$TARGETVARIANT" ]; then export GOARM=${TARGETVARIANT#v}; fi && \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o main .

# Final stage
FROM alpine:latest

WORKDIR /app

# Install ca-certificates for HTTPS requests
RUN apk --no-cache add ca-certificates

# Copy the binary from builder
COPY --from=builder /app/main .

# Ensure the binary is executable
RUN chmod +x /app/main

# Copy Swagger documentation files
COPY --from=builder /app/docs ./docs

# Copy views directory for metrics dashboard
COPY --from=builder /app/views ./views

# Copy any config files if needed
# COPY --from=builder /app/config ./config

# Expose the application port
EXPOSE 8080

# Command to run the application
CMD ["./main"]
