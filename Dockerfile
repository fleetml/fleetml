# The builder runs on the build machine's platform and cross-compiles for the
# image's (docker buildx --platform linux/amd64,linux/arm64, as the Release
# workflow's images job does). VERSION is the release without the v.
FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS builder
ARG TARGETOS TARGETARCH
ARG VERSION=dev
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ENV CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH
RUN go build -trimpath -ldflags "-s -w -X github.com/fleetml/fleetml/internal/version.Version=$VERSION" -o /fleetml-server ./cmd/server
RUN go build -trimpath -ldflags "-s -w -X github.com/fleetml/fleetml/internal/version.Version=$VERSION" -o /fleetml ./cmd/fleetml

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=builder /fleetml-server /usr/local/bin/fleetml-server
COPY --from=builder /fleetml /usr/local/bin/fleetml
COPY internal/db/migrations /migrations
EXPOSE 18080 19090
ENTRYPOINT ["fleetml-server"]
