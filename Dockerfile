# The builder runs on the build machine's platform and cross-compiles for the
# image's (docker buildx --platform linux/amd64,linux/arm64, as the Release
# workflow's images job does). VERSION is the release without the v; a build
# without it (docker compose in a clone) takes the version from the clone's
# tag, `git describe --tags`: 0.10.1 on the tag, 0.10.1-3-g<commit> after it.
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS builder
ARG TARGETOS TARGETARCH
ARG VERSION=dev
RUN apk add --no-cache git
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ENV CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH
RUN v="$VERSION"; if [ "$v" = dev ]; then v=$(git describe --tags --always 2>/dev/null || echo dev); fi; \
    for b in server:fleetml-server fleetml:fleetml; do \
      go build -trimpath -buildvcs=false -ldflags "-s -w -X github.com/fleetml/fleetml/internal/version.Version=$v" -o "/${b#*:}" "./cmd/${b%%:*}" || exit 1; \
    done

FROM alpine:3.19
ARG VERSION=dev
LABEL org.opencontainers.image.version=$VERSION
RUN apk add --no-cache ca-certificates
COPY --from=builder /fleetml-server /usr/local/bin/fleetml-server
COPY --from=builder /fleetml /usr/local/bin/fleetml
COPY internal/db/migrations /migrations
EXPOSE 18080 19090
ENTRYPOINT ["fleetml-server"]
