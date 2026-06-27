# Wukong Docker image with Chromium for website cloning.
# Multi-stage build: build Go binary → package with Chromium.
#
# Build:
#   docker build -t wukong .
#
# Run clone:
#   docker run --rm -v "$PWD/out:/out" wukong apps clone https://example.com
#
# Run session:
#   docker run --rm -v "$PWD/config:/root/.config/wukong" wukong session

# ---------------------------------------------------------------------------
# Stage 1: Build the Go binary
# ---------------------------------------------------------------------------
FROM golang:1.26-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /wukong ./cmd/wukong

# ---------------------------------------------------------------------------
# Stage 2: Runtime with Chromium
# ---------------------------------------------------------------------------
FROM alpine:3.21

# Install Chromium and dependencies.
# Uses Alpine's chromium package (includes headless support).
RUN apk add --no-cache \
    chromium \
    nss \
    freetype \
    harfbuzz \
    ttf-freefont \
    ca-certificates

# Tell chromedp / wukong where to find Chromium.
ENV CHROME_BIN=/usr/bin/chromium-browser
ENV KAGE_CHROME=/usr/bin/chromium-browser

# Chromium needs to run as non-root with --no-sandbox in Docker.
ENV CHROMIUM_FLAGS="--no-sandbox --disable-gpu --disable-dev-shm-usage"

COPY --from=builder /wukong /usr/local/bin/wukong

# Default data dir for clone output.
RUN mkdir -p /data /out
ENV HOME=/root

ENTRYPOINT ["wukong"]
CMD ["session"]
