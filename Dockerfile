# Build stage
FROM golang:1.23-alpine AS builder

WORKDIR /app

# Install dependencies
RUN apk add --no-cache git

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build binary
RUN CGO_ENABLED=0 GOOS=linux go build -o arbToolDBUpdater .

# Runtime stage
FROM alpine:latest

WORKDIR /app

# Install ca-certificates for HTTPS requests to exchanges
RUN apk --no-cache add ca-certificates tzdata

# Copy binary from builder
COPY --from=builder /app/arbToolDBUpdater .

# Copy SQL files (needed for scheduled jobs)
COPY --from=builder /app/db/queries ./db/queries

# Expose API port (optional, you said you won't use it)
EXPOSE 8082

CMD ["./arbToolDBUpdater"]
