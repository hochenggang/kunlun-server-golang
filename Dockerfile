# Build stage
FROM golang:1.22-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -ldflags "-s -w" -o kunlun-server

# Runtime stage
FROM alpine:latest

WORKDIR /app
COPY --from=builder /app/kunlun-server .

EXPOSE 8008

CMD ["./kunlun-server"]
