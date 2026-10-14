FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/scbz-server ./cmd/scbz-server

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata && adduser -D -u 10001 scbz && mkdir /data && chown scbz /data
USER scbz
WORKDIR /data
ENV SESSION_DB_PATH=/data/scbz_sessions.db \
    WEBHOOK_ENABLED=false \
    API_HOST=0.0.0.0
COPY --from=build /out/scbz-server /usr/local/bin/scbz-server
VOLUME /data
EXPOSE 8000
HEALTHCHECK --interval=30s --timeout=3s CMD wget -qO- http://127.0.0.1:${PORT:-8000}/health || exit 1
CMD ["scbz-server"]
