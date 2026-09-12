#!/bin/sh
set -e
mkdir -p /certs
if [ ! -f "${TLS_CERT:-/certs/server.crt}" ]; then
  openssl req -x509 -newkey rsa:2048 -nodes \
    -keyout /certs/server.key \
    -out /certs/server.crt \
    -days 365 \
    -subj "/CN=localhost"
fi
export TLS_CERT="${TLS_CERT:-/certs/server.crt}"
export TLS_KEY="${TLS_KEY:-/certs/server.key}"
export TLS_ADDR="${TLS_ADDR:-:8443}"
exec /app/world
