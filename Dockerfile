# ── Stage 1: Build ──────────────────────────────────────────────────────────
FROM golang:1.23-alpine AS build

WORKDIR /src

# Install git (needed for go mod download when module graph requires VCS)
RUN apk add --no-cache git

# Download dependencies first (layer-cached unless go.mod/go.sum change)
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build a static binary with debug symbols stripped
RUN CGO_ENABLED=0 GOOS=linux \
    go build -trimpath -ldflags='-s -w' -o /out/pkd ./cmd/pkd

# ── Stage 2: Runtime ─────────────────────────────────────────────────────────
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/pkd /pkd

USER nonroot:nonroot

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --retries=3 \
    CMD ["/pkd", "-healthcheck"]

ENTRYPOINT ["/pkd"]
