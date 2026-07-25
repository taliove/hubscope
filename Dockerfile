# Build the frontend
FROM node:22-alpine AS web
WORKDIR /build
RUN corepack enable
# Root pnpm-workspace.yaml carries allowBuilds (esbuild) — required by pnpm 11,
# which node:22-alpine's corepack resolves to and which fails the install on
# ignored build scripts (ERR_PNPM_IGNORED_BUILDS).
COPY pnpm-workspace.yaml ../
COPY web/package.json web/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY web/ ./
RUN pnpm build

# Build the backend (pure Go, no cgo)
FROM golang:1.26-alpine AS go
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY web/embed.go ./web/embed.go
COPY --from=web /build/dist ./web/dist
RUN CGO_ENABLED=0 go build -o /hubscope ./cmd/hubscope

# Runtime
FROM alpine:3.21
RUN adduser -D -u 10001 ahc && mkdir -p /data && chown ahc /data
USER ahc
COPY --from=go /hubscope /usr/local/bin/hubscope
ENV ADDR=:8080 DATA_PATH=/data/app.db
VOLUME /data
EXPOSE 8080
ENTRYPOINT ["hubscope"]
