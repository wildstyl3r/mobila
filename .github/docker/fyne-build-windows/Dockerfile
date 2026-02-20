FROM fyneio/fyne-cross-images:latest-windows

USER root

# Устанавливаем инструменты сборки
RUN apt-get update && apt-get install -y git make perl

# Сборка libvpx под Windows через Zig
RUN git clone --depth 1 https://chromium.googlesource.com /tmp/libvpx && \
    cd /tmp/libvpx && \
    CC="zig cc -target x86_64-windows-gnu" \
    CXX="zig c++ -target x86_64-windows-gnu" \
    AR="zig ar" \
    ./configure --target=generic-gnu --enable-static --disable-shared --prefix=/usr/x86_64-w64-mingw32 && \
    make -j$(nproc) && make install && \
    rm -rf /tmp/libvpx

# Настройка pkg-config, чтобы pion/mediadevices нашел библиотеку автоматически
ENV PKG_CONFIG_PATH_x86_64_w64_mingw32=/usr/x86_64-w64-mingw32/lib/pkgconfig
ENV PKG_CONFIG_ALLOW_CROSS=1

USER fyne
