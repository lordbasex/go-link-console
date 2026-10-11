# Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

# Cross-compiles the device for Windows (x86_64 or aarch64) with llvm-mingw.
# libvpx and libopus are built from source as static libraries, so the
# .exe needs no extra DLLs.
#
#   docker buildx build --build-arg ARCH=x86_64 --build-arg GOARCH=amd64 \
#     -f build/windows.Dockerfile --output type=local,dest=../dist/device/windows-amd64 .
# go-link HD's engine library, shipped next to the device: its sources come
# as the "golinkhd" build context (the Makefile's golinkhd-src); a plain
# `docker build` gets this empty stage and builds the device without it.
FROM scratch AS golinkhd

FROM mstorsjo/llvm-mingw:latest AS build
ARG ARCH=x86_64
ARG GOARCH=amd64
ARG OPUS_VERSION=1.5.2
ARG VPX_VERSION=1.15.0
ENV HOST=${ARCH}-w64-mingw32 PREFIX=/deps
RUN apt-get update && apt-get install -y --no-install-recommends \
      ca-certificates curl make pkg-config nasm yasm perl && rm -rf /var/lib/apt/lists/*
COPY --from=golang:1.26-bookworm /usr/local/go /usr/local/go
ENV PATH=/usr/local/go/bin:$PATH

# libopus, static. On ARM, Windows gives Opus no way to detect the CPU's
# features at run time (rtcd), so they are chosen when it is built.
RUN curl -fsSL https://downloads.xiph.org/releases/opus/opus-${OPUS_VERSION}.tar.gz | tar xz -C /tmp && \
    cd /tmp/opus-${OPUS_VERSION} && \
    RTCD="" && if [ "$ARCH" = aarch64 ]; then RTCD=--disable-rtcd; fi && \
    ./configure --host=$HOST --prefix=$PREFIX --enable-static --disable-shared \
      --disable-doc --disable-extra-programs $RTCD CC=$HOST-clang && \
    make -j"$(nproc)" && make install

# libvpx (VP8 only), static. Its encoder threads use winpthreads, hence
# -lpthread when the .exe is linked.
RUN curl -fsSL https://github.com/webmproject/libvpx/archive/refs/tags/v${VPX_VERSION}.tar.gz | tar xz -C /tmp && \
    cd /tmp/libvpx-${VPX_VERSION} && \
    case "$ARCH" in x86_64) T=x86_64-win64-gcc ;; aarch64) T=arm64-win64-gcc ;; esac && \
    CROSS=$HOST- CC=$HOST-clang CXX=$HOST-clang++ AR=llvm-ar LD=$HOST-clang STRIP=llvm-strip \
      ./configure --target=$T --prefix=$PREFIX --enable-static --disable-shared \
        --disable-examples --disable-tools --disable-docs --disable-unit-tests \
        --enable-vp8 --disable-vp9 && \
    make -j"$(nproc)" && make install

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=1 GOOS=windows GOARCH=$GOARCH CC=$HOST-clang CXX=$HOST-clang++ \
    PKG_CONFIG_PATH=$PREFIX/lib/pkgconfig PKG_CONFIG_LIBDIR=$PREFIX/lib/pkgconfig \
    CGO_LDFLAGS="-static -lpthread" \
    go build -trimpath -ldflags "-s -w -H windowsgui -X main.version=$VERSION" \
      -o /out/go-link-console.exe ./cmd/device
# go-link HD's engine as golinkhd.dll beside the .exe; only the API is
# exported (GOLINKHD_BUILD_SHARED marks it dllexport).
COPY --from=golinkhd / /golinkhd
RUN if [ -f /golinkhd/Makefile ]; then cd /golinkhd && \
      $HOST-clang -std=c99 -O2 -DNDEBUG -DGOLINKHD_BUILD_SHARED -Iinclude -Isrc -shared \
        -o /out/golinkhd.dll src/*.c; fi

FROM scratch AS export
COPY --from=build /out/ /
