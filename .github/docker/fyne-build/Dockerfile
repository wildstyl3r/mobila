FROM fyneio/fyne-cross-images:base
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
    libvpx-dev \
    libopus-dev \
    pkg-config && \
    rm -rf /var/lib/apt/lists/*