BINARY := bin/ai-hub-checker
GO := go

.PHONY: build test dev clean frontend-build frontend-test backend-build backend-test ensure-dist hooks

## build: frontend (vite) + backend single binary with embedded assets
build: frontend-build backend-build

## test: everything that must pass before any commit
test: backend-test frontend-test

backend-build: ensure-dist
	$(GO) build -o $(BINARY) ./cmd/ai-hub-checker

backend-test: ensure-dist
	$(GO) vet ./...
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
	$(GO) run ./cmd/ai-hub-checker

## ensure-dist: go:embed needs web/dist to exist even before the first vite build
ensure-dist:
	@mkdir -p web/dist
	@test -f web/dist/index.html || printf '<!doctype html><html><title>ai-hub-checker</title><body>frontend not built yet</body></html>' > web/dist/index.html

clean:
	rm -rf bin web/dist

## hooks: enable the pre-commit gate (.githooks/) for this clone — run once after clone
hooks:
	git config core.hooksPath .githooks
	@echo "pre-commit gate enabled: make test + secret scan run before every commit"
