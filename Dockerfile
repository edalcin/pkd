# ── Stage 1: Frontend build ───────────────────────────────────────────────────
FROM node:22-alpine AS frontend

WORKDIR /app/frontend

COPY frontend/package.json ./
RUN npm install

COPY frontend/ .
RUN npm run build
# Output: /app/internal/server/web/dist/


# ── Stage 2: Go build ─────────────────────────────────────────────────────────
FROM golang:1.25-alpine AS build

WORKDIR /src

RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=frontend /app/internal/server/web/dist/ ./internal/server/web/dist/

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags='-s -w' -o /out/pkd ./cmd/pkd


# ── Stage 3: Runtime ──────────────────────────────────────────────────────────
FROM gcr.io/distroless/static-debian12

COPY --from=build /out/pkd /pkd

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --retries=3 \
    CMD ["/pkd", "-healthcheck"]

ENTRYPOINT ["/pkd"]
