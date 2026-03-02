# ============ Build Stage ============
FROM golang:1.25-bookworm

RUN apt-get update && apt-get install -y \
    libgstreamer1.0-dev \
    libgstreamer-plugins-base1.0-dev \
    gstreamer1.0-plugins-base \
    gstreamer1.0-plugins-good \
    gstreamer1.0-alsa \
    gstreamer1.0-libav \
    alsa-utils \
    pkg-config \
    build-essential \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=1 go build -ldflags="-s -w" -o webrtc-worker ./cmd/main.go

# ============ Runtime Stage ============
# FROM debian:bookworm-slim

# RUN apt-get update && apt-get install -y --no-install-recommends \
#     gstreamer1.0-plugins-base \
#     gstreamer1.0-plugins-good \
#     gstreamer1.0-plugins-bad \
#     gstreamer1.0-plugins-ugly \
#     gstreamer1.0-libav \
#     ca-certificates \
#     && rm -rf /var/lib/apt/lists/* \
#     && apt-get clean

# Build non-root user
# RUN groupadd --gid 1000 appgroup && \
#     useradd --uid 1000 --gid appgroup syncrobotic

# WORKDIR /app

# COPY --from=builder --chown=syncrobotic:syncrobotic /app/webrtc-worker .

# USER syncrobotic

EXPOSE 18000

CMD ["./webrtc-worker"]
