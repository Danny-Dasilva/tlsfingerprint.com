#!/bin/bash
set -euo pipefail

DOMAIN=tlsfingerprint.com
LE_BASE=/var/lib/tlsfingerprint/letsencrypt
CERT_DIR=/var/lib/tlsfingerprint/certs
CONTAINER=tlsfingerprint
THRESHOLD_DAYS=30

EXPIRY=$(openssl x509 -in "$CERT_DIR/chain.pem" -noout -enddate | cut -d= -f2)
EXPIRY_TS=$(date -d "$EXPIRY" +%s)
NOW_TS=$(date +%s)
DAYS_LEFT=$(( (EXPIRY_TS - NOW_TS) / 86400 ))

echo "[$(date -u +%FT%TZ)] cert has $DAYS_LEFT days left (expires $EXPIRY)"

if [ "$DAYS_LEFT" -gt "$THRESHOLD_DAYS" ]; then
  echo "[$(date -u +%FT%TZ)] skip — renewal not yet due"
  exit 0
fi

echo "[$(date -u +%FT%TZ)] renewing..."
docker stop "$CONTAINER"
trap 'docker start "'"$CONTAINER"'" >/dev/null 2>&1 || true' EXIT

docker run --rm -p 80:80 \
  -v "$LE_BASE:/etc/letsencrypt" \
  -v "$LE_BASE/lib:/var/lib/letsencrypt" \
  -v "$LE_BASE/log:/var/log/letsencrypt" \
  certbot/certbot renew --no-random-sleep-on-renew --quiet

cp -f "$LE_BASE/live/$DOMAIN/fullchain.pem" "$CERT_DIR/chain.pem"
cp -f "$LE_BASE/live/$DOMAIN/privkey.pem"   "$CERT_DIR/key.pem"
chmod 600 "$CERT_DIR/key.pem"

docker start "$CONTAINER" >/dev/null
trap - EXIT

NEW_EXPIRY=$(openssl x509 -in "$CERT_DIR/chain.pem" -noout -enddate | cut -d= -f2)
echo "[$(date -u +%FT%TZ)] renewed — new expiry: $NEW_EXPIRY"
