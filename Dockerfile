FROM node:22-alpine AS ui-build
WORKDIR /app
COPY frontend/package*.json ./
RUN npm ci
COPY frontend .
RUN if [ -d build/_app/immutable ]; then \
        mkdir -p /tmp/previous-immutable && \
        cp -a build/_app/immutable/. /tmp/previous-immutable/; \
    fi && \
    npm run build && \
    if [ -d /tmp/previous-immutable ]; then \
        mkdir -p build/_app/immutable && \
        cp -an /tmp/previous-immutable/. build/_app/immutable/; \
    fi

FROM golang:1.25-alpine AS api-build
WORKDIR /app
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend .
RUN CGO_ENABLED=0 go build -o cryptorum ./cmd/server

FROM debian:bookworm-slim
ARG CALIBRE_URL=https://download.calibre-ebook.com/9.7.0/calibre-9.7.0-x86_64.txz
ARG CALIBRE_TARBALL=calibre-9.7.0-x86_64.txz
RUN apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
    ca-certificates \
    tzdata \
    wget \
    xz-utils \
    ffmpeg \
    fonts-liberation \
    poppler-utils \
    && rm -rf /var/lib/apt/lists/* /var/cache/apt/archives/*
COPY calibre-cache/ /tmp/calibre-cache/
RUN mkdir -p /opt/calibre && \
    if [ -f "/tmp/calibre-cache/$CALIBRE_TARBALL" ]; then \
        cp "/tmp/calibre-cache/$CALIBRE_TARBALL" /tmp/calibre.txz; \
    else \
        wget -O /tmp/calibre.txz "$CALIBRE_URL"; \
    fi && \
    tar -xJf /tmp/calibre.txz -C /opt/calibre && \
    ln -sf "$(find /opt/calibre -type f -name ebook-convert | head -n 1)" /usr/local/bin/ebook-convert && \
    ln -sf "$(find /opt/calibre -type f -name ebook-meta | head -n 1)" /usr/local/bin/ebook-meta && \
    rm -f /tmp/calibre.txz
WORKDIR /app
COPY --from=ui-build /app/build ./static
COPY --from=api-build /app/cryptorum .
EXPOSE 6060
CMD ["./cryptorum"]
