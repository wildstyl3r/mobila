FROM fyneio/fyne-cross-images:windows

USER root
RUN apt-get update && apt-get install -y git make perl
RUN git clone --depth 1 https://chromium.googlesource.com/webm/libvpx /tmp/libvpx && \
    cd /tmp/libvpx && \
    CC="zig cc -target x86_64-windows-gnu" \
    CXX="zig c++ -target x86_64-windows-gnu" \
    AR="zig ar" \
    ./configure --target=generic-gnu --enable-static --disable-shared --prefix=/usr/x86_64-w64-mingw32 && \
    make -j$(nproc) && make install && \
    rm -rf /tmp/libvpx

ENV PKG_CONFIG_PATH=/usr/x86_64-w64-mingw32/lib/pkgconfig
ENV PKG_CONFIG_ALLOW_CROSS=1

USER fyne
