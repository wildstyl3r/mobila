FROM fyneio/fyne-cross:base-latest
RUN sed -i 's/deb.debian.org/archive.debian.org/g' /etc/apt/sources.list && \
    sed -i 's|security.debian.org/debian-security|archive.debian.org/debian-security|g' /etc/apt/sources.list && \
    sed -i '/stretch-updates/d' /etc/apt/sources.list
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
    libvpx-dev \
    libopus-dev \
    pkg-config && \
    rm -rf /var/lib/apt/lists/*
RUN rm -rf /usr/local/go && curl -L https://go.dev/dl/go1.24.6.linux-amd64.tar.gz | tar -C /usr/local -xz
ENV PATH="/usr/local/go/bin:${PATH}"