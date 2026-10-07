FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /fleetml-server ./cmd/server
RUN CGO_ENABLED=0 go build -o /fleetml ./cmd/fleetml

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=builder /fleetml-server /usr/local/bin/fleetml-server
COPY --from=builder /fleetml /usr/local/bin/fleetml
COPY internal/db/migrations /migrations
EXPOSE 18080 19090
ENTRYPOINT ["fleetml-server"]
