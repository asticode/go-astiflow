FROM debian:12.5

RUN apt-get update

RUN apt-get install -y \
	build-essential \
	nasm \
	git \
	pkg-config \
	libpng-dev \
	libx264-dev

RUN \
	mkdir -p /opt/ffmpeg/src
    
WORKDIR /opt/ffmpeg/src

RUN \
	git clone https://github.com/FFmpeg/FFmpeg /opt/ffmpeg/src && \
	git checkout n5.1.2

RUN \
	./configure --prefix=.. --enable-libx264 --enable-gpl && \
	make && \
	make install

ADD https://dl.google.com/go/go1.22.0.linux-amd64.tar.gz /tmp/go.tar.gz
RUN tar -C /opt -xzf /tmp/go.tar.gz

ENV GOCACHE=/opt/asticode/go-astiflow/internal/testleak/tmp/gocache
ENV GOMODCACHE=/opt/asticode/go-astiflow/internal/testleak/tmp/gomodcache
ENV CGO_LDFLAGS=-L/opt/ffmpeg/lib/
ENV CGO_CFLAGS=-I/opt/ffmpeg/include/
ENV PKG_CONFIG_PATH=/opt/ffmpeg/lib/pkgconfig

RUN apt-get install -y \
	valgrind
    
WORKDIR /opt/asticode/go-astiflow

CMD ["./internal/testleak/run.sh"]