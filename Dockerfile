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
# The workspaces are shared through codeforge-ws (10010) with the worker
# (setgid, so new entries join the group; the Go Core creates files
# group-writable). Agent tools run as their tenant's tool UID (KI-96,
# ADR-018): the root is 2771 (tools pass through, cannot list tenant IDs or
# create anything), and the Go Core gives each tenant directory POSIX ACLs
# for its tenant's tool UID before any clone or init (workspace.tool_acls).
RUN addgroup -S -g 10001 codeforge && adduser -S -u 10001 -G codeforge codeforge && \
    addgroup -S -g 10010 codeforge-ws && addgroup codeforge codeforge-ws && \
    mkdir -p /data/workspaces && chown codeforge:codeforge /data && \
    chown codeforge:codeforge-ws /data/workspaces && chmod 2771 /data/workspaces

# Per-tenant tool identities (KI-96): tenant directories get ACLs and tool
# work carries the tenant's tool UID, which the worker image requires.
ENV CODEFORGE_WORKSPACE_TOOL_ACLS=required

COPY --from=build /codeforge /usr/local/bin/codeforge

USER codeforge

EXPOSE 8080

# FIX-112: Container health check for orchestrators (Docker Compose, K8s).
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget -q --spider http://localhost:8080/health || exit 1

ENTRYPOINT ["codeforge"]
