FROM node:24-bookworm-slim AS web-build

WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web ./
RUN npm run build

FROM golang:1.26.5-alpine AS go-build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/continuity-server ./cmd/continuity-server

FROM alpine:3.23

RUN apk add --no-cache ca-certificates tzdata
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
