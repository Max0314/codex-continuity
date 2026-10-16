FROM node:24.21.0-bookworm-slim AS web-build

WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web ./
RUN npm run build

FROM golang:1.27.2-alpine AS go-build
ENV GOPROXY=https://goproxy.cn,direct

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN go test ./... && go vet ./... && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/continuity-server ./cmd/continuity-server

FROM alpine:3.23

RUN sed -i 's|https://dl-cdn.alpinelinux.org/alpine|https://mirrors.cloud.tencent.com/alpine|g' /etc/apk/repositories \
    && apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=go-build /out/continuity-server /app/continuity-server
COPY --from=web-build /src/web/dist /app/web
RUN addgroup -S continuity && adduser -S -G continuity continuity \
    && mkdir -p /data /downloads \
    && chown -R continuity:continuity /data /downloads /app
USER continuity

ENV CONTINUITY_ADDR=:8080
ENV CONTINUITY_DATA_DIR=/data
ENV CONTINUITY_WEB_DIR=/app/web
ENV CONTINUITY_DOWNLOAD_DIR=/downloads

EXPOSE 8080
CMD ["/app/continuity-server"]
