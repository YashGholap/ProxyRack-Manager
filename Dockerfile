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
COPY --from=builder /app/proxyrack-manager /usr/local/bin/proxyrack-manager
EXPOSE 8080
ENTRYPOINT ["proxyrack-manager"]