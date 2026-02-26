# syntax=docker/dockerfile:1

# --- Build stage ---
FROM golang:1.25-alpine AS build

RUN apk add --no-cache git ca-certificates

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /codeforge ./cmd/codeforge

# --- Runtime stage ---
FROM alpine:3.21

RUN apk add --no-cache git ca-certificates tzdata

RUN addgroup -S codeforge && adduser -S codeforge -G codeforge

COPY --from=build /codeforge /usr/local/bin/codeforge

USER codeforge

EXPOSE 8080

ENTRYPOINT ["codeforge"]
