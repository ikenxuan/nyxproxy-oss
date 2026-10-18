# syntax=docker/dockerfile:1

FROM --platform=$BUILDPLATFORM node:24-alpine AS web
WORKDIR /app/web
RUN corepack enable && corepack prepare pnpm@12.14.0 --activate
COPY web/package.json web/pnpm-lock.yaml web/pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile
COPY web/ ./
RUN pnpm build

FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS builder

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown
ARG TARGETARCH

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=web /app/web/out ./internal/webui/dist

RUN CGO_ENABLED=0 GOOS=linux GOARCH=$TARGETARCH go build -trimpath \
    -ldflags="-s -w \
      -X github.com/jannik-schroeder/nyxproxy-oss/pkg/version.Version=${VERSION} \
      -X github.com/jannik-schroeder/nyxproxy-oss/pkg/version.Commit=${COMMIT} \
      -X github.com/jannik-schroeder/nyxproxy-oss/pkg/version.BuildTime=${BUILD_TIME}" \
    -o /out/nyxproxy ./cmd/proxy

FROM alpine:3.22

RUN apk add --no-cache iproute2 ca-certificates tzdata \
    && mkdir -p /data \
    && chmod 700 /data

COPY --from=builder /out/nyxproxy /usr/local/bin/nyxproxy
COPY docker/entrypoint.sh /usr/local/bin/entrypoint.sh
COPY docker/config.template.yaml /etc/nyxproxy/config.template.yaml

RUN chmod +x /usr/local/bin/entrypoint.sh

WORKDIR /data
VOLUME ["/data"]

HEALTHCHECK --interval=30s --timeout=5s --start-period=30s --retries=3 \
  CMD ["/usr/local/bin/nyxproxy", "-healthcheck", "-config", "/data/config.yaml"]

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
