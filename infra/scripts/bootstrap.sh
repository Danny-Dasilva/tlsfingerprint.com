#!/bin/bash
# Bootstrap a fresh Container-Optimized OS instance to run tlsfingerprint
# with auto-renewing Let's Encrypt certs.
#
# Usage: sudo bash bootstrap.sh <DOMAIN> <EMAIL> [<DOCKER_IMAGE>]
#
# Idempotent: re-running on an already-bootstrapped host is safe.

set -euo pipefail

DOMAIN="${1:-}"
EMAIL="${2:-}"
IMAGE="${3:-gcr.io/scrolller/tlsfingerprint:latest}"

if [ -z "$DOMAIN" ] || [ -z "$EMAIL" ]; then
  echo "usage: sudo bash bootstrap.sh <DOMAIN> <EMAIL> [<DOCKER_IMAGE>]" >&2
  exit 2
fi

STATE=/var/lib/tlsfingerprint
CERT_DIR="$STATE/certs"
LE_BASE="$STATE/letsencrypt"
CONTAINER=tlsfingerprint
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

echo "[bootstrap] state dir: $STATE"
mkdir -p "$STATE" "$CERT_DIR" "$LE_BASE"/{lib,log} "$STATE/config"
touch "$STATE/blockedIPs"

if [ ! -f "$STATE/config.json" ]; then
  echo "[bootstrap] writing default config.json"
  cat > "$STATE/config.json" <<EOF
{
  "log_to_db": false,
  "tls_port": "443",
  "http_port": "80",
  "cert_file": "certs/chain.pem",
  "key_file": "certs/key.pem",
  "host": "0.0.0.0",
  "http_redirect": "https://$DOMAIN",
  "mongo_url": "",
  "mongo_database": "TrackMe",
  "mongo_collection": "requests",
  "mongo_log_ips": false,
  "device": "eth0",
  "cors_key": "X-CORS"
}
EOF
fi

# --- DNS sanity check: refuse to issue if domain doesn't resolve to us
LOCAL_IP=$(curl -fsS -H 'Metadata-Flavor: Google' \
  http://metadata.google.internal/computeMetadata/v1/instance/network-interfaces/0/access-configs/0/external-ip)
RESOLVED_IP=$(getent ahostsv4 "$DOMAIN" | awk 'NR==1{print $1}')
echo "[bootstrap] domain=$DOMAIN resolved=$RESOLVED_IP local=$LOCAL_IP"
if [ "$RESOLVED_IP" != "$LOCAL_IP" ]; then
  echo "[bootstrap] FATAL: $DOMAIN resolves to $RESOLVED_IP but this VM is $LOCAL_IP." >&2
  echo "[bootstrap] Point DNS at $LOCAL_IP and retry." >&2
  exit 3
fi

# --- Issue cert if we don't already have one
if [ ! -s "$LE_BASE/live/$DOMAIN/fullchain.pem" ]; then
  echo "[bootstrap] no existing cert — issuing via certbot certonly"
  docker run --rm -p 80:80 \
    -v "$LE_BASE:/etc/letsencrypt" \
    -v "$LE_BASE/lib:/var/lib/letsencrypt" \
    -v "$LE_BASE/log:/var/log/letsencrypt" \
    certbot/certbot certonly --standalone -d "$DOMAIN" \
      --agree-tos -m "$EMAIL" --non-interactive
else
  echo "[bootstrap] cert already exists at $LE_BASE/live/$DOMAIN — skipping issuance"
fi

cp -f "$LE_BASE/live/$DOMAIN/fullchain.pem" "$CERT_DIR/chain.pem"
cp -f "$LE_BASE/live/$DOMAIN/privkey.pem"   "$CERT_DIR/key.pem"
chmod 600 "$CERT_DIR/key.pem"

# --- Install renewal script
echo "[bootstrap] installing renewal script and systemd units"
install -m 0755 -o root -g root "$SCRIPT_DIR/renew-cert.sh" "$STATE/renew-cert.sh" 2>/dev/null || \
  cp "$SCRIPT_DIR/renew-cert.sh" "$STATE/renew-cert.sh"
cp "$SCRIPT_DIR/../systemd/tls-cert-renew.service" /etc/systemd/system/tls-cert-renew.service 2>/dev/null || \
  cp "$SCRIPT_DIR/tls-cert-renew.service" /etc/systemd/system/tls-cert-renew.service
cp "$SCRIPT_DIR/../systemd/tls-cert-renew.timer"   /etc/systemd/system/tls-cert-renew.timer 2>/dev/null || \
  cp "$SCRIPT_DIR/tls-cert-renew.timer"   /etc/systemd/system/tls-cert-renew.timer

systemctl daemon-reload
systemctl enable --now tls-cert-renew.timer

# --- Pull and (re)start the app container
echo "[bootstrap] pulling $IMAGE"
docker pull "$IMAGE"

if docker ps -a --format '{{.Names}}' | grep -qx "$CONTAINER"; then
  echo "[bootstrap] removing existing container"
  docker rm -f "$CONTAINER" >/dev/null
fi

echo "[bootstrap] starting $CONTAINER"
docker run -d --name "$CONTAINER" \
  --restart unless-stopped \
  --cap-add NET_ADMIN --cap-add NET_RAW \
  -p 80:80 -p 443:443 -p 443:443/udp \
  -v "$CERT_DIR:/app/certs" \
  -v "$STATE/config.json:/app/config.json" \
  -v "$STATE/blockedIPs:/app/blockedIPs" \
  "$IMAGE"

echo "[bootstrap] done."
echo "[bootstrap] timer status:"
systemctl list-timers tls-cert-renew.timer --no-pager
echo "[bootstrap] cert dates:"
openssl x509 -in "$CERT_DIR/chain.pem" -noout -subject -dates
