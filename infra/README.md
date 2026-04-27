# Infrastructure

Terraform + bootstrap scripts to spin up a tlsfingerprint instance on GCP.

## Layout

```
infra/
├── terraform/        # GCE instance, static IP, firewall rules
├── scripts/
│   ├── bootstrap.sh  # one-shot: cert + container + systemd timer
│   └── renew-cert.sh # daily renewal, invoked by the timer
└── systemd/
    ├── tls-cert-renew.service
    └── tls-cert-renew.timer
```

## Spin up a new server

### 1. Provision GCP resources

```bash
cd infra/terraform
cp terraform.tfvars.example terraform.tfvars
# edit project, name, etc.

terraform init
terraform apply
```

Outputs include the public IP and the exact `gcloud compute ssh` /
`scp` commands you'll need.

### 2. Point DNS at the instance

Create an A record for your domain pointing at `terraform output public_ip`.

**Cloudflare:** keep proxying OFF (grey cloud). If Cloudflare terminates
TLS, the service captures Cloudflare's fingerprint instead of real
clients — defeating the whole point.

Wait for DNS to propagate before continuing (the bootstrap script
fails fast if the domain doesn't resolve to the new VM).

### 3. Bootstrap the VM

The Terraform output prints the exact commands. The shape is:

```bash
gcloud compute scp \
  infra/scripts/bootstrap.sh \
  infra/scripts/renew-cert.sh \
  infra/systemd/tls-cert-renew.service \
  infra/systemd/tls-cert-renew.timer \
  <name>:~/ \
  --zone=<zone> --project=<project>

gcloud compute ssh <name> --zone=<zone> --project=<project> \
  --command='sudo bash ~/bootstrap.sh <DOMAIN> <EMAIL> [<DOCKER_IMAGE>]'
```

Bootstrap is idempotent — safe to re-run if you change config or images.

## Adopt the existing production instance

The current `tlsfingerprint` VM in project `scrolller`, zone
`us-central1-a`, IP `34.170.213.141` was created by hand. To bring it
under Terraform without recreating it:

```bash
cd infra/terraform
terraform init
terraform import google_compute_address.this projects/scrolller/regions/us-central1/addresses/tlsfingerprint-ip
terraform import google_compute_instance.this projects/scrolller/zones/us-central1-a/instances/tlsfingerprint
terraform import google_compute_firewall.http  projects/scrolller/global/firewalls/allow-http
terraform import google_compute_firewall.https projects/scrolller/global/firewalls/allow-https
terraform import google_compute_firewall.quic  projects/scrolller/global/firewalls/allow-quic
terraform plan   # review carefully before applying
```

The firewall rule names in the imports above (`allow-http`,
`allow-https`, `allow-quic`) match what's currently in GCP. The
Terraform code uses prefixed names (`<name>-allow-http`) for new
instances. Adjust the resource `name` field in `main.tf` to match the
existing names if you want to import without renaming, or rename the
existing rules in GCP first.

## What the bootstrap does

1. Creates `/var/lib/tlsfingerprint/{certs,config,letsencrypt}` and
   writes a default `config.json`.
2. Verifies DNS points at the VM (refuses to continue otherwise — Let's
   Encrypt rate limits punish blind retries).
3. Issues a Let's Encrypt cert via `certbot certonly --standalone`
   (skipped if a cert already exists).
4. Installs `renew-cert.sh` and the systemd `.service` + `.timer` units.
5. Enables the timer (daily, with up to 1h jitter — only acts when ≤30
   days remain).
6. Pulls the docker image and starts the container with `--cap-add
   NET_ADMIN --cap-add NET_RAW` and ports 80/tcp, 443/tcp, 443/udp.

## Why systemd, not cron

Container-Optimized OS doesn't ship cron — it's a stripped-down
container host. systemd is the only scheduler available.

`/var/lib` is also mounted `noexec` on COS, which is why the service
unit invokes `/bin/bash <script>` rather than exec'ing the script
directly.

## Operating the timer

```bash
# When does it next run?
sudo systemctl list-timers tls-cert-renew.timer

# What did the last few runs do?
sudo tail /var/log/tls-renew.log

# Force a renewal check now (no-op if >30 days remain)
sudo systemctl start tls-cert-renew.service
```

To force an actual renewal for testing, edit `renew-cert.sh` to bump
`THRESHOLD_DAYS` above current days-remaining, run the service once,
then revert. Don't add `--force-renewal` to the certbot call in cron —
Let's Encrypt rate limits will bite.

## Cross-project image access

`gcr.io/scrolller/tlsfingerprint:latest` is in the `scrolller` GCR.
A new instance in a different project needs either:

- the new project's compute service account granted
  `roles/storage.objectViewer` on `gs://artifacts.scrolller.appspot.com`, or
- the image pushed/copied into the new project's GCR/Artifact Registry,
  and the bootstrap invoked with that image as the third arg.
