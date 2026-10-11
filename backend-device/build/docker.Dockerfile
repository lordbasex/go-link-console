# Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

# The device as a container: the headless build (no window) with its web
# panel on port 7373. Everything it keeps (device.json, the emulator core,
# saved games, history, thumbnails and, by default, the ROMs) lives in one
# volume, /data. The emulator core is downloaded the first time, never
# bundled; the ROMs are the host's own.
#
#   docker buildx build -f build/docker.Dockerfile -t go-link-console --load .
#   docker run -d --name go-link --network host -v go-link:/data \
#     -v /path/to/roms:/data/go-link-console/roms go-link-console
# The licenses (LICENSE, THIRD_PARTY_NOTICES.md) come from the repository
# root: the Makefile passes them as the "legal" build context. A plain
# `docker build` gets this empty stage instead.
FROM scratch AS legal

# go-link HD's engine library, shipped next to the device: its sources come
# as the "golinkhd" build context (the Makefile's golinkhd-src); a plain
# `docker build` gets this empty stage and builds the device without it.
FROM scratch AS golinkhd

FROM golang:1.26-bookworm AS build
RUN apt-get update && apt-get install -y --no-install-recommends \
      pkg-config libvpx-dev libopus-dev && rm -rf /var/lib/apt/lists/*
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
# libvpx and libopus go inside the binary (see linux.Dockerfile).
RUN mkdir /static && ln -s /usr/lib/*-linux-gnu/libvpx.a /usr/lib/*-linux-gnu/libopus.a /static/ && \
    CGO_ENABLED=1 CGO_LDFLAGS="-L/static" go build -trimpath -tags headless \
      -ldflags "-s -w -X main.version=$VERSION" -o /out/go-link-console ./cmd/device
COPY --from=golinkhd / /golinkhd
RUN mkdir -p /out/lib && if [ -f /golinkhd/Makefile ]; then make -s -B -C /golinkhd && cp /golinkhd/libgolinkhd.so /out/lib/; fi

FROM debian:bookworm-slim
# CA certificates: wss:// to the signaling server and the core download.
# libstdc++: the libretro core (a C++ library loaded at run time).
RUN apt-get update && apt-get install -y --no-install-recommends \
      ca-certificates libstdc++6 && rm -rf /var/lib/apt/lists/* && \
    useradd --create-home --home-dir /data --uid 1000 golink && \
    mkdir -p /data/go-link-console/cores /data/go-link-console/roms /data/go-link-console/thumbnails/MAME \
      /data/go-link-console/saves /data/go-link-console/logs && chown -R golink:golink /data/go-link-console
# The folders exist in the image, owned by golink: a new volume gets them
# with that owner. Otherwise mounting the ROMs at /data/go-link-console/roms makes
# Docker create go-link/ as root, and the device cannot write its core.
COPY --from=build /out/go-link-console /usr/local/bin/go-link-console
# go-link HD's engine beside the program, where the device looks for it.
COPY --from=build /out/lib/ /usr/local/bin/
COPY --from=legal / /usr/share/doc/go-link/
USER golink
ENV HOME=/data
VOLUME /data
WORKDIR /data
# 7373/tcp: the web panel. WebRTC uses UDP: with --network host nothing
# else is needed; on a bridge network publish a fixed --udp-port and
# --announce the host's LAN address (see the README).
EXPOSE 7373/tcp
ENTRYPOINT ["go-link-console", "--headless", "--config", "/data/device.json"]
