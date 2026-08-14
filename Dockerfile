# Build stage
FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o proxyrack-manager ./cmd/

# Runtime stage
FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=builder /app/pool-manager /usr/local/bin/pool-manager
EXPOSE 8555
ENTRYPOINT ["proxyrack-manager"]