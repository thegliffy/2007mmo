FROM golang:1.22-alpine AS build
WORKDIR /src
RUN apk add --no-cache git ca-certificates
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/world ./cmd/world \
 && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/bots ./cmd/bots \
 && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/admin ./cmd/admin

FROM alpine:3.20
RUN apk add --no-cache ca-certificates openssl wget
WORKDIR /app
COPY --from=build /out/world /app/world
COPY --from=build /out/bots /app/bots
COPY --from=build /out/admin /app/admin
COPY client /app/web
COPY migrations /app/migrations
COPY deploy/entrypoint.sh /app/entrypoint.sh
RUN chmod +x /app/entrypoint.sh

# Run as a normal user. The entrypoint generates a self-signed cert into
# /certs, so that has to be writable before dropping privileges. Both
# listen ports are above 1024, so nothing here needs root.
RUN addgroup -S hollow && adduser -S -G hollow -h /app hollow \
 && mkdir -p /certs \
 && chown -R hollow:hollow /certs /app
USER hollow
ENV WEB_DIR=/app/web \
    HTTP_ADDR=:8080 \
    TLS_ADDR=:8443 \
    TICK_MS=600
EXPOSE 8080 8443
HEALTHCHECK --interval=5s --timeout=3s --retries=12 CMD wget -qO- http://127.0.0.1:8080/health || exit 1
ENTRYPOINT ["/app/entrypoint.sh"]
