FROM golang:1.14-alpine AS dependencies

RUN apk add --no-cache \
    git \
    curl \
    tar

WORKDIR /tmp/svgcleaner

RUN curl -fsL "https://github.com/RazrFalcon/svgcleaner-gui/releases/download/v0.9.5/svgcleaner_linux_x86_64_0.9.5.tar.gz" | tar -xz \
    && chmod +x svgcleaner \
    && mv svgcleaner /usr/bin/svgcleaner \
    && rm -rf /tmp/svgcleaner

WORKDIR /tmp/build

COPY go.mod .
COPY go.sum .

RUN go mod download

COPY . .

RUN go build -o ./out/resizer .

FROM alpine

ENV STORAGE_LOCAL_PREFIX=/var/www/cache/

RUN apk add --no-cache \
        imagemagick \
        pngquant \
        jpegoptim \
        gifsicle \
        libwebp-tools \
        file && \
    mkdir -p /var/www/cache

COPY --from=dependencies /usr/bin/svgcleaner /usr/bin/svgcleaner

WORKDIR /var/www/

COPY --from=dependencies /tmp/build/out/resizer .
RUN chmod +x /var/www/resizer

ENV PORT=3000
EXPOSE $PORT

CMD ["/var/www/resizer"]