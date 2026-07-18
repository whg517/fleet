# syntax=docker/dockerfile:1

# ──────────────────────────────────────────────
# Stage 1: Go builder
# ──────────────────────────────────────────────
FROM golang:1.26-alpine AS builder

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /build

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-s -w -X main.Version=$(git describe --tags --always 2>/dev/null || echo dev)" \
    -o /out/fleet-server \
    ./cmd/server

# ──────────────────────────────────────────────
# Stage 2: Frontend builder
# ──────────────────────────────────────────────
FROM node:22-alpine AS frontend-builder

WORKDIR /build

COPY web/package.json web/package-lock.json* ./
# Use npm ci if lock exists, otherwise npm install
RUN if [ -f package-lock.json ]; then npm ci; else npm install; fi

COPY web/ .
RUN npm run build

# ──────────────────────────────────────────────
# Stage 3: Runtime (distroless)
# ──────────────────────────────────────────────
FROM gcr.io/distroless/static:nonroot

LABEL org.opencontainers.image.title="Fleet" \
      org.opencontainers.image.description="Fleet — DevOps deployment platform" \
      org.opencontainers.image.source="https://github.com/whg517/fleet"

# Copy Go binary
COPY --from=builder /out/fleet-server /fleet-server

# Copy Next.js standalone output
COPY --from=frontend-builder /build/.next/standalone /app/
COPY --from=frontend-builder /build/.next/static /app/.next/static
COPY --from=frontend-builder /build/public /app/public

# Non-root user (distroless static:nonroot provides user 65532)
USER 65532:65532

EXPOSE 8080

ENTRYPOINT ["/fleet-server"]
