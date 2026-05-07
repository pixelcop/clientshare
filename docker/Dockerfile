# Frontend build
FROM --platform=$BUILDPLATFORM oven/bun:1 AS web-base
WORKDIR /app

ENV DEBIAN_FRONTEND=noninteractive

# Sentry vars
ARG VITE_SENTRY_DSN=
ENV VITE_SENTRY_DSN=${VITE_SENTRY_DSN}

# need to update ca-certificates for sentry sourcemap upload
RUN apt update && apt install -y ca-certificates

COPY web/package.json web/bun.lock ./web/
RUN --mount=type=cache,target=/root/.bun/install/cache \
    cd web && bun install --frozen-lockfile
COPY web ./web

FROM web-base AS web-build

RUN mkdir -p /app/internal/clientshare/web

ARG SENTRY_RELEASE=
ENV SENTRY_RELEASE=${SENTRY_RELEASE}
RUN --mount=type=cache,target=/root/.bun/install/cache \
    cd web && bun run --bun build && bun run --bun build:emails

FROM web-base AS web-test

RUN --mount=type=cache,target=/root/.bun/install/cache \
    cd web && bun run vitest run

# Backend build
FROM --platform=$BUILDPLATFORM golang:1-alpine AS go-base

ARG TARGETOS
ARG TARGETARCH

ENV GOCACHE=/tmp/.cache/go-build

WORKDIR /app
RUN apk add --no-cache build-base sqlite-dev
RUN chown -R nobody /app
USER nobody
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod,uid=65534 \
    go mod download
COPY --exclude=web/ . .
COPY --from=web-build /app/internal/static/dist ./internal/static/dist
COPY --from=web-build /app/internal/services/email/generated ./internal/services/email/generated

FROM go-base AS go-build

# Sentry vars
ARG SENTRY_RELEASE=
ARG BUILD_TIME=
ARG COMMIT_HASH=
ENV BUILD_TIME=${BUILD_TIME}
ENV COMMIT_HASH=${COMMIT_HASH}
ENV SENTRY_RELEASE=${SENTRY_RELEASE}

RUN --mount=type=cache,target=/go/pkg/mod,uid=65534 \
    --mount=type=cache,target=/tmp/.cache/go-build,uid=65534 \
    make build BUILD_PATH=/app/bin/ GO_BUILD_ARCH="GOOS=${TARGETOS} GOARCH=${TARGETARCH}"

FROM go-base AS go-test

RUN --mount=type=cache,target=/go/pkg/mod,uid=65534 \
    --mount=type=cache,target=/tmp/.cache/go-build,uid=65534 \
    make test

# Runtime image
FROM alpine:3.20 AS runtime
RUN apk add --no-cache ca-certificates sqlite-libs mailcap
USER nobody
WORKDIR /app

COPY --from=go-build /app/bin/clientshare ./clientshare
COPY --from=go-build /app/bin/importclients ./importclients
COPY --from=go-build /app/bin/goose ./goose
COPY internal/database/migrations migrations
COPY web/public public
COPY docker/entrypoint.sh /entrypoint.sh

RUN mkdir -p ./data ./storage
EXPOSE 8080
CMD ["/entrypoint.sh"]
