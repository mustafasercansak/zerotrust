FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o server ./cmd/server

FROM alpine:3.21
WORKDIR /app

RUN apk add --no-cache ca-certificates \
    && addgroup -S app \
    && adduser -S app -G app

COPY --from=builder /app/server .
COPY --from=builder /app/migrations ./migrations

USER app

EXPOSE 8080

ENTRYPOINT ["./server"]
