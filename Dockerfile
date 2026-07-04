# syntax=docker/dockerfile:1.6
# =============================================================================
# Multi-architecture build for MediaStationGo.
#
# Stage 1 (frontend) :  Node 20  -> static SPA bundle
# Stage 2 (backend)  :  Go 1.25  -> single static binary (CGO_ENABLED=0)
# Stage 3 (runtime)  :  Alpine 3.23 -> ffmpeg + tzdata + non-root user
#
# Build:
#   docker buildx build --platform linux/amd64,linux/arm64 \
#     --build-arg VERSION=MediaStationGo-v0.1.16 -t mediastation-go:latest --push .
#
# Optional Intel VAAPI/QSV runtime packages:
#   docker buildx build --build-arg WITH_VAAPI=true ...
# =============================================================================

# ---- Stage 1: frontend (always build on the host architecture) -------------
FROM --platform=$BUILDPLATFORM node:20-alpine AS frontend
ARG NPM_CONFIG_REGISTRY=https://registry.npmjs.org/
WORKDIR /app/web
COPY web/package*.json ./
RUN --mount=type=cache,target=/root/.npm \
    npm ci --registry="${NPM_CONFIG_REGISTRY}"
COPY web/ .
RUN npm run build

# ---- Stage 2: backend (cross-compiled to TARGETPLATFORM) -------------------
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS backend
ARG TARGETOS
ARG TARGETARCH
ARG GOPROXY=https://proxy.golang.org,direct
ARG VERSION=dev
ENV GOPROXY=${GOPROXY}
WORKDIR /app
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download
COPY . .
COPY --from=frontend /app/web/dist ./web/dist
RUN --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o mediastation-go ./cmd/server

# ---- Stage 3: runtime ------------------------------------------------------
FROM alpine:3.23
ARG WITH_VAAPI=false
# Default runtime keeps only the packages needed by normal deployments.
# VAAPI/mesa drivers pull a large graphics dependency tree, so they are opt-in
# for users who explicitly build an Intel hardware-acceleration image.
# NVENC requires the proprietary NVIDIA Container Toolkit on the host only.
RUN apk add --no-cache \
        ffmpeg \
        docker-cli \
        tzdata \
        ca-certificates \
        su-exec \
    && if [ "$WITH_VAAPI" = "true" ]; then \
        if [ "$(apk --print-arch)" = "x86_64" ]; then \
            apk add --no-cache intel-media-driver libva-utils mesa-va-gallium; \
        else \
            apk add --no-cache libva-utils mesa-va-gallium || true; \
        fi; \
    fi \
    && rm -rf /var/cache/apk/*

# Non-root user for the long-running process.
RUN addgroup -S mediastation && adduser -S mediastation -G mediastation

WORKDIR /app
COPY --from=backend /app/mediastation-go /usr/local/bin/mediastation-go
COPY --from=frontend /app/web/dist /app/web/dist

RUN mkdir -p /data /cache /media \
    && chown -R mediastation:mediastation /data /cache /media

# Default environment (overridable via docker-compose / `docker run -e`).
ENV MEDIASTATION_APP_PORT=8080 \
    MEDIASTATION_APP_DATA_DIR=/data \
    MEDIASTATION_APP_WEB_DIR=/app/web/dist \
    MEDIASTATION_DATABASE_DB_PATH=/data/mediastation.db \
    MEDIASTATION_CACHE_CACHE_DIR=/cache \
    MEDIASTATION_LOGGING_LEVEL=info \
    TZ=Asia/Shanghai

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
    CMD busybox wget -q --spider http://127.0.0.1:8080/api/health || exit 1

# Tiny entrypoint that lets us run as a NAS host UID/GID via PUID/PGID without
# rewriting /etc/passwd or /etc/group on every container start.
COPY docker-entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

CMD ["/entrypoint.sh"]
