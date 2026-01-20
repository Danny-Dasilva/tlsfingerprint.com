# GCP Deployment for TLS Fingerprint Server

Deploy the TLS fingerprinting server to Google Cloud Platform using Compute Engine.

## Prerequisites

1. [Google Cloud SDK](https://cloud.google.com/sdk/docs/install) installed and configured
2. Docker installed locally
3. A GCP project with billing enabled
4. (Optional) A domain name pointing to your server

## Quick Start

```bash
# Enable required APIs
gcloud services enable compute.googleapis.com containerregistry.googleapis.com

# Deploy (uses e2-small ~$12/month)
./deploy.sh YOUR_PROJECT_ID

# Or with custom instance name and zone
./deploy.sh YOUR_PROJECT_ID tlsfingerprint us-west1-a
```

## What Gets Deployed

| Resource | Description |
|----------|-------------|
| GCE Instance | Container-Optimized OS running Docker |
| Firewall Rules | TCP 80, TCP 443, UDP 443 (for HTTP/3) |
| Container | TLS fingerprint server with NET_ADMIN/NET_RAW capabilities |

## Architecture

```
Internet
    |
    v
[GCP Firewall: 80, 443/tcp, 443/udp]
    |
    v
[GCE VM: Container-Optimized OS]
    |
    +-- [Docker: --network=host --cap-add=NET_ADMIN,NET_RAW]
            |
            +-- [tlsfingerprint container]
                    |
                    +-- Port 80: HTTP redirect
                    +-- Port 443/tcp: HTTPS (TLS fingerprinting)
                    +-- Port 443/udp: HTTP/3 (QUIC)
```

## TLS Fingerprinting Requirements

For TLS fingerprinting with packet capture to work, the container needs:

| Requirement | Why | How |
|-------------|-----|-----|
| `--network=host` | Access real client IPs, not NAT'd container IPs | Startup script uses `--network=host` |
| `NET_ADMIN` | Network configuration for packet capture | `--cap-add=NET_ADMIN` |
| `NET_RAW` | Raw socket access for libpcap | `--cap-add=NET_RAW` |
| Correct interface | GCP uses `ens4` not `eth0` | Auto-detected in startup script |

### Without Packet Capture

If you don't need TCP/IP header details (TTL, window size), you can disable packet capture:

1. Set `"device": ""` in config.json
2. Remove `--cap-add` and `--network=host` flags
3. Use standard port mapping `-p 80:80 -p 443:443 -p 443:443/udp`

This still provides full TLS fingerprinting (JA3, JA4, PeetPrint, Akamai H2).

## SSL Certificates

The startup script generates self-signed certificates for testing. For production:

### Option 1: Upload existing certificates

```bash
# SSH into the instance
gcloud compute ssh tlsfingerprint --zone=us-central1-a

# Upload your certificates
sudo mkdir -p /opt/tlsfingerprint/certs
sudo nano /opt/tlsfingerprint/certs/chain.pem  # paste certificate
sudo nano /opt/tlsfingerprint/certs/key.pem    # paste private key
sudo chmod 600 /opt/tlsfingerprint/certs/key.pem

# Restart the container
docker restart tlsfingerprint
```

### Option 2: Use Let's Encrypt with certbot

```bash
# SSH into instance
gcloud compute ssh tlsfingerprint --zone=us-central1-a

# Stop the container temporarily
docker stop tlsfingerprint

# Install and run certbot (requires domain pointed to this IP)
sudo apt-get update && sudo apt-get install -y certbot
sudo certbot certonly --standalone -d yourdomain.com

# Copy certificates
sudo cp /etc/letsencrypt/live/yourdomain.com/fullchain.pem /opt/tlsfingerprint/certs/chain.pem
sudo cp /etc/letsencrypt/live/yourdomain.com/privkey.pem /opt/tlsfingerprint/certs/key.pem

# Restart container
docker start tlsfingerprint
```

## Cost Estimates

| Instance Type | vCPU | Memory | Monthly Cost |
|---------------|------|--------|--------------|
| e2-micro | 0.25 | 1 GB | ~$6.12 (free tier eligible) |
| e2-small (default) | 0.5 | 2 GB | ~$12.23 |
| e2-medium | 1 | 4 GB | ~$24.46 |

To use a different instance type:

```bash
MACHINE_TYPE=e2-micro ./deploy.sh YOUR_PROJECT_ID
```

## Useful Commands

```bash
# View container logs
gcloud compute ssh tlsfingerprint --zone=us-central1-a -- docker logs -f tlsfingerprint

# SSH into instance
gcloud compute ssh tlsfingerprint --zone=us-central1-a

# Restart container
gcloud compute ssh tlsfingerprint --zone=us-central1-a -- docker restart tlsfingerprint

# Check container status
gcloud compute ssh tlsfingerprint --zone=us-central1-a -- docker ps

# View firewall rules
gcloud compute firewall-rules list --filter="name~allow-(http|https|quic)"

# Delete everything
gcloud compute instances delete tlsfingerprint --zone=us-central1-a
gcloud compute firewall-rules delete allow-http allow-https allow-quic
```

## Troubleshooting

### Container not starting

```bash
gcloud compute ssh tlsfingerprint --zone=us-central1-a
docker logs tlsfingerprint
```

### Port not accessible

Check firewall rules and ensure VM has correct tags:
```bash
gcloud compute instances describe tlsfingerprint --zone=us-central1-a --format='get(tags.items)'
# Should show: http-server, https-server
```

### TLS fingerprinting not working

1. Check if packet capture is enabled in config.json (`device` should be `ens4`)
2. Verify container has NET_ADMIN/NET_RAW capabilities:
   ```bash
   docker inspect tlsfingerprint | grep -A 10 CapAdd
   ```
3. Ensure `--network=host` is being used (not port mapping)

### Wrong network interface

The startup script auto-detects the interface, but you can manually check:
```bash
gcloud compute ssh tlsfingerprint --zone=us-central1-a
ip link show  # Usually ens4 on GCP
```

## Alternative: Cloud Run (No Packet Capture)

If you don't need packet capture, Cloud Run is simpler and cheaper:

```bash
# Build and push
docker build -t gcr.io/YOUR_PROJECT/tlsfingerprint .
docker push gcr.io/YOUR_PROJECT/tlsfingerprint

# Deploy to Cloud Run (no packet capture, but TLS fingerprinting works)
gcloud run deploy tlsfingerprint \
    --image gcr.io/YOUR_PROJECT/tlsfingerprint \
    --port 443 \
    --allow-unauthenticated \
    --region us-central1
```

Note: Cloud Run does not support UDP (HTTP/3) or NET_ADMIN capabilities.
