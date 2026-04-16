# ── Stage 1: Frontend build ───────────────────────────────────────────────────
# Builds the Svelte 5 + Vite frontend and outputs to /app/frontend/dist/
FROM node:22-alpine AS frontend

WORKDIR /app/frontend

# Install dependencies first (layer-cached unless package*.json change)
COPY frontend/package.json frontend/package-lock.json* ./
RUN npm ci --prefer-offline

# Build the Svelte app
COPY frontend/ .
RUN npm run build
# Output: /app/frontend/dist/ (→ ../internal/server/web/dist relative to vite.config)
# But in Docker we copy from the absolute dist/ path


# ── Stage 2: Go build ─────────────────────────────────────────────────────────
# BUILDPLATFORM = plataforma do runner CI (sempre linux/amd64).
# TARGETARCH/TARGETOS = plataforma alvo (amd64 ou arm64).
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS build

ARG TARGETOS=linux
ARG TARGETARCH=amd64

WORKDIR /src

RUN apk add --no-cache git

# Download dependencies (layer-cached unless go.mod/go.sum change)
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Copy compiled Svelte build into the Go embed directory
# vite.config.js outputs to internal/server/web/dist when run locally,
# but in Docker the frontend stage outputs to /app/frontend/dist.
COPY --from=frontend /app/frontend/dist/ ./internal/server/web/dist/

# Build a static binary with debug symbols stripped
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags='-s -w' -o /out/pkd ./cmd/pkd


# ── Stage 3: Runtime ──────────────────────────────────────────────────────────
FROM gcr.io/distroless/static-debian12

COPY --from=build /out/pkd /pkd

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --retries=3 \
    CMD ["/pkd", "-healthcheck"]

ENTRYPOINT ["/pkd"]
