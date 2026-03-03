FROM golang:1.25-bookworm 

RUN apt-get update && apt-get install -y --no-install-recommends gosu \
    pkg-config \
    libgstreamer1.0-dev \
    libgstreamer-plugins-base1.0-dev \
    gstreamer1.0-plugins-good \
    gstreamer1.0-plugins-bad \
    gstreamer1.0-plugins-ugly \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /src

ENTRYPOINT [ "go"]