# ── Stage 1: Frontend build ───────────────────────────────────────────────────
# Builds the Svelte 5 + Vite frontend.
# vite.config.js uses outDir '../internal/server/web/dist', which resolves
# to /app/internal/server/web/dist/ when run from /app/frontend/.
FROM node:22-alpine AS frontend

WORKDIR /app/frontend

# Install dependencies (no lockfile required — npm install generates one)
COPY frontend/package.json ./
RUN npm install

# Copy full frontend source and build
COPY frontend/ .
RUN npm run build
# Output lands at: /app/internal/server/web/dist/


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

# Copy compiled Svelte build into the Go embed directory.
# The Vite outDir '../internal/server/web/dist' relative to /app/frontend/
# resolves to /app/internal/server/web/dist/ in the frontend stage.
COPY --from=frontend /app/internal/server/web/dist/ ./internal/server/web/dist/

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
