GO ?= go
PNPM ?= pnpm

VERSION ?= dev
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_TIME ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w \
	-X github.com/jannik-schroeder/nyxproxy-oss/pkg/version.Version=$(VERSION) \
	-X github.com/jannik-schroeder/nyxproxy-oss/pkg/version.Commit=$(COMMIT) \
	-X github.com/jannik-schroeder/nyxproxy-oss/pkg/version.BuildTime=$(BUILD_TIME)

.PHONY: web embed build test lint docker clean

web:
	cd web && $(PNPM) install --frozen-lockfile && $(PNPM) build

embed: web
	rm -rf internal/webui/dist
	mkdir -p internal/webui/dist
	cp -R web/out/. internal/webui/dist/
	touch internal/webui/dist/.gitkeep

build: embed
	mkdir -p dist
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o dist/nyxproxy ./cmd/proxy

test:
	$(GO) test -race ./...
	cd web && $(PNPM) test

lint:
	$(GO) vet ./...
	cd web && $(PNPM) lint

docker:
	docker build -t nyxproxy-oss:local .

clean:
	rm -rf dist internal/webui/dist/* web/out web/.next
