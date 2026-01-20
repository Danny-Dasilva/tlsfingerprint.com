#!/bin/bash
# Deploy TLS Fingerprint server to GCP Compute Engine
# Usage: ./deploy.sh <project-id> [instance-name] [zone] [domain]

set -e

PROJECT_ID="${1:?Usage: ./deploy.sh <project-id> [instance-name] [zone] [domain]}"
INSTANCE_NAME="${2:-tlsfingerprint}"
ZONE="${3:-us-central1-a}"
DOMAIN="${4:-tlsfingerprint.com}"
MACHINE_TYPE="${MACHINE_TYPE:-e2-small}"
IMAGE_NAME="tlsfingerprint"

echo "=== TLS Fingerprint GCP Deployment ==="
echo "Project: $PROJECT_ID"
echo "Instance: $INSTANCE_NAME"
echo "Zone: $ZONE"
echo "Domain: $DOMAIN"
echo "Machine Type: $MACHINE_TYPE"
echo ""

# Set project
gcloud config set project "$PROJECT_ID"

# Step 1: Build and push Docker image to GCR
echo "=== Step 1: Building and pushing Docker image ==="
cd "$(dirname "$0")/../.."
docker build -t "gcr.io/$PROJECT_ID/$IMAGE_NAME:latest" .
docker push "gcr.io/$PROJECT_ID/$IMAGE_NAME:latest"
echo "Image pushed to gcr.io/$PROJECT_ID/$IMAGE_NAME:latest"

# Step 2: Create firewall rules if they don't exist
echo ""
echo "=== Step 2: Creating firewall rules ==="

# HTTP
gcloud compute firewall-rules describe allow-http --project="$PROJECT_ID" &>/dev/null || \
gcloud compute firewall-rules create allow-http \
    --project="$PROJECT_ID" \
    --network=default \
    --direction=ingress \
    --action=allow \
    --rules=tcp:80 \
    --source-ranges=0.0.0.0/0 \
    --target-tags=http-server \
    --description="Allow HTTP traffic"

# HTTPS (TCP)
gcloud compute firewall-rules describe allow-https --project="$PROJECT_ID" &>/dev/null || \
gcloud compute firewall-rules create allow-https \
    --project="$PROJECT_ID" \
    --network=default \
    --direction=ingress \
    --action=allow \
    --rules=tcp:443 \
    --source-ranges=0.0.0.0/0 \
    --target-tags=https-server \
    --description="Allow HTTPS traffic"

# QUIC/HTTP3 (UDP)
gcloud compute firewall-rules describe allow-quic --project="$PROJECT_ID" &>/dev/null || \
gcloud compute firewall-rules create allow-quic \
    --project="$PROJECT_ID" \
    --network=default \
    --direction=ingress \
    --action=allow \
    --rules=udp:443 \
    --source-ranges=0.0.0.0/0 \
    --target-tags=https-server \
    --description="Allow QUIC/HTTP3 traffic"

echo "Firewall rules configured"

# Step 3: Create or update the VM instance
echo ""
echo "=== Step 3: Creating/Updating VM instance ==="

# Check if instance exists
if gcloud compute instances describe "$INSTANCE_NAME" --zone="$ZONE" --project="$PROJECT_ID" &>/dev/null; then
    echo "Instance exists. Updating startup script and restarting..."

    # Update metadata with new startup script and domain
    gcloud compute instances add-metadata "$INSTANCE_NAME" \
        --zone="$ZONE" \
        --project="$PROJECT_ID" \
        --metadata=DOMAIN="$DOMAIN" \
        --metadata-from-file=startup-script="$(dirname "$0")/startup-script.sh"

    # Restart to apply changes
    gcloud compute instances reset "$INSTANCE_NAME" \
        --zone="$ZONE" \
        --project="$PROJECT_ID"
else
    echo "Creating new instance..."

    gcloud compute instances create "$INSTANCE_NAME" \
        --project="$PROJECT_ID" \
        --zone="$ZONE" \
        --machine-type="$MACHINE_TYPE" \
        --image-family=cos-stable \
        --image-project=cos-cloud \
        --boot-disk-size=10GB \
        --boot-disk-type=pd-standard \
        --tags=http-server,https-server \
        --metadata-from-file=startup-script="$(dirname "$0")/startup-script.sh" \
        --metadata=IMAGE_TAG="gcr.io/$PROJECT_ID/$IMAGE_NAME:latest",PROJECT_ID="$PROJECT_ID",DOMAIN="$DOMAIN" \
        --scopes=storage-ro,logging-write,monitoring-write
fi

# Step 4: Get the external IP
echo ""
echo "=== Step 4: Getting external IP ==="
EXTERNAL_IP=$(gcloud compute instances describe "$INSTANCE_NAME" \
    --zone="$ZONE" \
    --project="$PROJECT_ID" \
    --format='get(networkInterfaces[0].accessConfigs[0].natIP)')

echo ""
echo "=== Deployment Complete ==="
echo "Instance: $INSTANCE_NAME"
echo "External IP: $EXTERNAL_IP"
echo ""
echo "Test endpoints:"
echo "  curl -k https://$EXTERNAL_IP/api/all"
echo "  curl http://$EXTERNAL_IP"
echo ""
echo "SSH into instance:"
echo "  gcloud compute ssh $INSTANCE_NAME --zone=$ZONE --project=$PROJECT_ID"
echo ""
echo "View container logs:"
echo "  gcloud compute ssh $INSTANCE_NAME --zone=$ZONE --project=$PROJECT_ID -- docker logs -f tlsfingerprint"
echo ""
echo "Domain configured: $DOMAIN"
echo "IMPORTANT: Update your DNS to point $DOMAIN to $EXTERNAL_IP"
echo "IMPORTANT: Upload real SSL certificates to /var/lib/tlsfingerprint/certs/ on the VM"
