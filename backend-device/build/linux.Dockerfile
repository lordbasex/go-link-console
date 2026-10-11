# Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

# Builds the device for Linux (amd64 or arm64) with its C libraries: libvpx
# and libopus (static) for the encoders, OpenGL, X11 and Wayland for the Fyne window. The
# target platform runs through buildx (QEMU for the other architecture).
#
#   docker buildx build --platform linux/arm64 --build-arg TAGS=headless \
#     -f build/linux.Dockerfile --output type=local,dest=../dist/device/linux-arm64 .
# go-link HD's engine library, shipped next to the device: its sources come
# as the "golinkhd" build context (the Makefile's golinkhd-src); a plain
# `docker build` gets this empty stage and builds the device without it.
FROM scratch AS golinkhd

FROM golang:1.26-bookworm AS build
RUN apt-get update && apt-get install -y --no-install-recommends \
      pkg-config libvpx-dev libopus-dev libgl1-mesa-dev xorg-dev \
      libwayland-dev libxkbcommon-dev wayland-protocols && \
    rm -rf /var/lib/apt/lists/*
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG TAGS=""
ARG VERSION=dev
# libvpx and libopus go inside the binary (a folder with only their .a
# files goes first in the search), so it runs on any distribution whatever
# libvpx version it ships. The window's GL, X11 and Wayland libraries stay
# shared: every Linux desktop has them.
RUN mkdir /static && ln -s /usr/lib/*-linux-gnu/libvpx.a /usr/lib/*-linux-gnu/libopus.a /static/ && \
    CGO_ENABLED=1 CGO_LDFLAGS="-L/static" go build -trimpath -tags "$TAGS" \
      -ldflags "-s -w -X main.version=$VERSION" -o /out/go-link-console ./cmd/device
COPY --from=golinkhd / /golinkhd
RUN if [ -f /golinkhd/Makefile ]; then make -s -B -C /golinkhd && cp /golinkhd/libgolinkhd.so /out/; fi

# Only the binary and the engine library leave the build (docker buildx --output type=local).
FROM scratch AS export
COPY --from=build /out/ /
