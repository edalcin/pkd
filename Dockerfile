# ── Stage 1: Build ──────────────────────────────────────────────────────────
# BUILDPLATFORM = plataforma do runner CI (sempre linux/amd64).
# TARGETARCH/TARGETOS = plataforma alvo (amd64 ou arm64).
# Assim o Go compila nativamente para arm64 sem QEMU — muito mais rápido.
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS build

ARG TARGETOS=linux
ARG TARGETARCH=amd64

WORKDIR /src

# Install git (needed for go mod download when module graph requires VCS)
RUN apk add --no-cache git

# Download dependencies first (layer-cached unless go.mod/go.sum change)
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build a static binary with debug symbols stripped.
# CGO_ENABLED=0 garante binário estático puro.
# GOOS/GOARCH vêm dos ARGs do BuildKit — cross-compilation nativa, sem QEMU.
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags='-s -w' -o /out/pkd ./cmd/pkd

# ── Stage 2: Runtime ─────────────────────────────────────────────────────────
FROM gcr.io/distroless/static-debian12

COPY --from=build /out/pkd /pkd

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --retries=3 \
    CMD ["/pkd", "-healthcheck"]

ENTRYPOINT ["/pkd"]
