FROM golang:1.18-alpine AS builder

WORKDIR /app

# Copy go mod and sum files
COPY go.mod ./

# Download all dependencies
RUN go mod download

# Copy the source code
COPY . .

# Build the applications
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o api-gateway ./cmd/api-gateway/main.go
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o queue-manager ./cmd/queue-manager/main.go
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o scheduler ./cmd/scheduler/main.go

# Use a smaller image for the final stage
FROM alpine:latest

WORKDIR /app

# Install necessary packages
RUN apk --no-cache add ca-certificates tzdata

# Copy the binaries from the builder stage
COPY --from=builder /app/api-gateway /app/
COPY --from=builder /app/queue-manager /app/
COPY --from=builder /app/scheduler /app/

# Copy configuration files
COPY configs/config.yaml /app/configs/

# Set timezone
ENV TZ=Asia/Shanghai

# Expose ports
EXPOSE 8080

# Command to run when starting the container
CMD ["/app/api-gateway"]