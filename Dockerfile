# Build the frontend
FROM node:22-alpine AS web
WORKDIR /build
RUN corepack enable
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
RUN CGO_ENABLED=0 go build -o /ai-hub-checker ./cmd/ai-hub-checker

# Runtime
FROM alpine:3.21
RUN adduser -D -u 10001 ahc && mkdir -p /data && chown ahc /data
USER ahc
COPY --from=go /ai-hub-checker /usr/local/bin/ai-hub-checker
ENV ADDR=:8080 DATA_PATH=/data/app.db
VOLUME /data
EXPOSE 8080
ENTRYPOINT ["ai-hub-checker"]
