# syntax=docker/dockerfile:1

# --- Build stage ---
FROM golang:1.25-alpine AS build

ARG APP_VERSION=dev
ARG GIT_SHA=unknown

RUN apk add --no-cache git ca-certificates

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath \
    -ldflags="-s -w -X github.com/Strob0t/CodeForge/internal/version.Version=${APP_VERSION} -X github.com/Strob0t/CodeForge/internal/version.GitSHA=${GIT_SHA}" \
    -o /codeforge ./cmd/codeforge

# --- Runtime stage ---
FROM alpine:3.21

ARG APP_VERSION=dev
ARG GIT_SHA=unknown

LABEL org.opencontainers.image.version="${APP_VERSION}" \
      org.opencontainers.image.revision="${GIT_SHA}"

RUN apk add --no-cache git ca-certificates tzdata

# Fixed UID/GID shared with the worker image: both write the shared workspaces
# volume. /data is created here so fresh named volumes mounted at /data and
# /data/workspaces are initialized with this ownership (the rootfs is read-only
# in production).
RUN addgroup -S -g 10001 codeforge && adduser -S -u 10001 -G codeforge codeforge && \
    mkdir -p /data/workspaces && chown -R codeforge:codeforge /data

COPY --from=build /codeforge /usr/local/bin/codeforge

USER codeforge

EXPOSE 8080

# FIX-112: Container health check for orchestrators (Docker Compose, K8s).
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget -q --spider http://localhost:8080/health || exit 1

ENTRYPOINT ["codeforge"]
