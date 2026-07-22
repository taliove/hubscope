BINARY := bin/hubscope
GO := go

.PHONY: build test dev clean fmt lint package frontend-build frontend-test backend-build backend-test ensure-dist hooks

## build: frontend (vite) + backend single binary with embedded assets
build: frontend-build backend-build

## test: everything that must pass before any commit
test: lint backend-test frontend-test

## fmt: auto-format Go sources (frontend formats via its own tooling if present)
fmt:
	$(GO)fmt -w cmd internal

## lint: static checks — unformatted Go and vet findings are hard failures
lint:
	@test -z "$$($(GO)fmt -l cmd internal)" || { $(GO)fmt -l cmd internal; echo "gofmt: run 'make fmt'"; exit 1; }
	$(GO) vet ./...

## package: build + tarball the single binary with deploy docs
package: build
	mkdir -p dist
	tar -czf dist/hubscope-$$(git describe --tags --always --dirty 2>/dev/null || echo dev).tar.gz $(BINARY) Dockerfile docs/deployment.md

backend-build: ensure-dist
	$(GO) build -o $(BINARY) ./cmd/hubscope

backend-test: ensure-dist
	$(GO) test ./...

frontend-build:
	@if [ -f web/package.json ]; then \
		cd web && pnpm install && pnpm build; \
	else \
		echo "skip: web/ not initialized yet"; \
	fi

frontend-test:
	@if [ -f web/package.json ]; then \
		cd web && pnpm install && pnpm typecheck && pnpm build; \
	else \
		echo "skip: web/ not initialized yet"; \
	fi

## dev: run backend locally (stub frontend if not built)
dev: ensure-dist
	$(GO) run ./cmd/hubscope

## ensure-dist: go:embed needs web/dist to exist even before the first vite build
ensure-dist:
	@mkdir -p web/dist
	@test -f web/dist/index.html || printf '<!doctype html><html><title>hubscope</title><body>frontend not built yet</body></html>' > web/dist/index.html

clean:
	rm -rf bin web/dist dist

## hooks: enable the pre-commit gate (.githooks/) for this clone — run once after clone
hooks:
	git config core.hooksPath .githooks
	@echo "pre-commit gate enabled: make test + secret scan run before every commit"
