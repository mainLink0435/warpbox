# Warpbox — Multi-stage Docker build
#
# Stage 1: Build the Go binary with CGO (required by mattn/go-sqlite3).
FROM golang:1.26-alpine AS build

RUN apk add --no-cache gcc musl-dev sqlite-dev

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 go build -tags netgo -ldflags='-s -w -extldflags=-static' \
    -o /warpbox ./cmd/warpbox/

# ---------------------------------------------------------------------------
# Stage 2: Minimal runtime image.
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata

COPY --from=build /warpbox /usr/local/bin/warpbox

VOLUME /data
EXPOSE 1412

ENTRYPOINT ["warpbox", "--config", "/data/config.yml", "--db", "/data/warpbox.db"]