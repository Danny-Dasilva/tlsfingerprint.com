#!/bin/bash
# GCP Compute Engine startup script for TLS Fingerprint Server
# This script runs on VM boot to configure and start the container

set -e

# Get metadata values (set by deploy.sh)
PROJECT_ID=$(curl -sf "http://metadata.google.internal/computeMetadata/v1/project/project-id" -H "Metadata-Flavor: Google")
IMAGE_TAG=$(curl -sf "http://metadata.google.internal/computeMetadata/v1/instance/attributes/IMAGE_TAG" -H "Metadata-Flavor: Google" || echo "gcr.io/${PROJECT_ID}/tlsfingerprint:latest")
DOMAIN=$(curl -sf "http://metadata.google.internal/computeMetadata/v1/instance/attributes/DOMAIN" -H "Metadata-Flavor: Google" || echo "tlsfingerprint.com")

# Configuration
CONTAINER_NAME="tlsfingerprint"
DATA_DIR="/var/lib/tlsfingerprint"

# Configure iptables to allow HTTP/HTTPS traffic (COS has restrictive defaults)
iptables -C INPUT -p tcp --dport 80 -j ACCEPT 2>/dev/null || iptables -A INPUT -p tcp --dport 80 -j ACCEPT
iptables -C INPUT -p tcp --dport 443 -j ACCEPT 2>/dev/null || iptables -A INPUT -p tcp --dport 443 -j ACCEPT
iptables -C INPUT -p udp --dport 443 -j ACCEPT 2>/dev/null || iptables -A INPUT -p udp --dport 443 -j ACCEPT

echo "=== TLS Fingerprint Startup Script ==="
echo "Project: $PROJECT_ID"
echo "Image: $IMAGE_TAG"
echo "Domain: $DOMAIN"

# Detect the primary network interface (GCP uses eth0 on COS)
NETWORK_INTERFACE=$(ip route | grep default | awk '{print $5}')
echo "Detected network interface: $NETWORK_INTERFACE"

# Create config directory
mkdir -p "$DATA_DIR/certs"
mkdir -p "$DATA_DIR/config"

# Generate config.json if it doesn't exist
if [ ! -f "$DATA_DIR/config/config.json" ]; then
    cat > "$DATA_DIR/config/config.json" << EOF
{
  "log_to_db": false,
  "tls_port": "443",
  "http_port": "80",
  "cert_file": "certs/chain.pem",
  "key_file": "certs/key.pem",
  "host": "0.0.0.0",
  "http_redirect": "https://${DOMAIN}",
  "mongo_url": "",
  "mongo_database": "TrackMe",
  "mongo_collection": "requests",
  "mongo_log_ips": false,
  "device": "${NETWORK_INTERFACE}",
  "cors_key": "X-CORS"
}
EOF
    echo "Generated config.json with interface: $NETWORK_INTERFACE"
fi

# Create empty blockedIPs if it doesn't exist
touch "$DATA_DIR/blockedIPs"

# Check if certificates exist
if [ ! -f "$DATA_DIR/certs/chain.pem" ]; then
    echo "WARNING: No certificates found at $DATA_DIR/certs/"
    echo "Generating self-signed certs for testing..."

    # Generate self-signed certs for testing (replace with real certs in production)
    openssl req -x509 -newkey rsa:2048 \
        -keyout "$DATA_DIR/certs/key.pem" \
        -out "$DATA_DIR/certs/chain.pem" \
        -sha256 -days 365 -nodes \
        -subj "/CN=${DOMAIN}"
    echo "Generated self-signed certificates"
fi

# Configure Docker to authenticate with GCR
echo "Configuring Docker for GCR..."
docker-credential-gcr configure-docker --registries=gcr.io 2>/dev/null || true

# Pull latest image
echo "Pulling image: $IMAGE_TAG"
docker pull "$IMAGE_TAG" || {
    echo "Failed to pull image, trying with gcloud auth..."
    gcloud auth configure-docker gcr.io --quiet 2>/dev/null || true
    docker pull "$IMAGE_TAG"
}

# Stop and remove existing container if running
docker stop "$CONTAINER_NAME" 2>/dev/null || true
docker rm "$CONTAINER_NAME" 2>/dev/null || true

# Run the container with full network access for TLS fingerprinting
echo "Starting container..."
docker run -d \
    --name "$CONTAINER_NAME" \
    --restart unless-stopped \
    --network=host \
    --cap-add=NET_ADMIN \
    --cap-add=NET_RAW \
    -v "$DATA_DIR/config/config.json:/app/config.json:ro" \
    -v "$DATA_DIR/certs:/app/certs:ro" \
    -v "$DATA_DIR/blockedIPs:/app/blockedIPs" \
    "$IMAGE_TAG"

echo "=== TLS Fingerprint server started successfully ==="
echo "Container logs: docker logs -f $CONTAINER_NAME"
