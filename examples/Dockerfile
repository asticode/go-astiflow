FROM debian:12.5

RUN apt-get update

RUN apt-get install -y \
	build-essential \
	nasm \
	git \
	pkg-config \
    libopus-dev \
    libvpx-dev \
    libopenjp2-7-dev \
    libmp3lame-dev \
    libx265-dev \
	libx264-dev \
	libpng-dev

RUN \
	mkdir -p /opt/ffmpeg/src
    
WORKDIR /opt/ffmpeg/src

RUN \
	git clone https://github.com/FFmpeg/FFmpeg /opt/ffmpeg/src && \
	git checkout n5.1.2

RUN \
	./configure \
        --prefix=.. \
        --enable-pthreads \
        --enable-gpl \
        --enable-libopus \
        --enable-libvpx \
        --enable-libopenjpeg \
        --enable-libmp3lame \
        --enable-libx265 \
        --enable-libx264 \
        && \
	make && \
	make install

ADD https://dl.google.com/go/go1.22.0.linux-amd64.tar.gz /tmp/go.tar.gz
RUN tar -C /opt -xzf /tmp/go.tar.gz

ENV GOCACHE=/opt/asticode/go-astiflow/examples/tmp/gocache
ENV GOMODCACHE=/opt/asticode/go-astiflow/examples/tmp/gomodcache
ENV CGO_LDFLAGS=-L/opt/ffmpeg/lib/
ENV CGO_CFLAGS=-I/opt/ffmpeg/include/
ENV PKG_CONFIG_PATH=/opt/ffmpeg/lib/pkgconfig
    
WORKDIR /opt/asticode/go-astiflow

CMD [ "/opt/go/bin/go" ]