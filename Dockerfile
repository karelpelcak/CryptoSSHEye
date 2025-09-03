# Build stage
FROM golang:1.22 AS builder
WORKDIR /app
COPY . .
RUN go mod tidy
RUN go build -o crypto-ssh-eye .

# Run stage
FROM debian:bullseye-slim
WORKDIR /app
COPY --from=builder /app/crypto-ssh-eye /app/
EXPOSE 2222
CMD ["/app/crypto-ssh-eye"]
