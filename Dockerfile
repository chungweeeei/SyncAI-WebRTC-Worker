# Build environment (Go toolchain + GStreamer dev libraries).
# Used by docker-compose `build-lib` to compile libsyncai_worker.so.
#
# The base is the DEPLOYMENT distro, not a Go image, and that is the whole
# point of this file. go-gst links GStreamer through cgo, so the .so carries
# undefined symbols resolved against whatever libgstreamer-1.0 the host
# process has. Building on golang:1.25-bookworm (GStreamer 1.22) produced a
# library that dlopen'd on the robot's Ubuntu 22.04 / GStreamer 1.20 container
# as:
#
#   undefined symbol: gst_debug_message_get_id
#
# — at load time, in the Python host, long after any build succeeded. Match the
# runtime's distro and the symbol set matches with it. Change this base only
# together with the robot container's base image.
FROM ubuntu:22.04 AS builder

# The Go toolchain is copied in rather than apt-installed: Ubuntu 22.04 ships
# Go 1.18 and go.mod asks for 1.25.
COPY --from=golang:1.25-bookworm /usr/local/go /usr/local/go
ENV PATH=/usr/local/go/bin:$PATH

RUN apt-get update && apt-get install -y --no-install-recommends \
    gosu \
    ca-certificates \
    gcc \
    libc6-dev \
    pkg-config \
    libgstreamer1.0-dev \
    libgstreamer-plugins-base1.0-dev \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download
