# Build stage
FROM golang:1.23-alpine AS builder

WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o main .

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
