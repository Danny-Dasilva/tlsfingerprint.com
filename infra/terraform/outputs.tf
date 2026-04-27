output "instance_name" {
  value = google_compute_instance.this.name
}

output "zone" {
  value = google_compute_instance.this.zone
}

output "public_ip" {
  value = google_compute_address.this.address
}

output "ssh_command" {
  value = "gcloud compute ssh ${google_compute_instance.this.name} --zone=${var.zone} --project=${var.project}"
}

output "next_steps" {
  description = "What to do after `terraform apply`"
  value       = <<-EOT

    Public IP: ${google_compute_address.this.address}

    1. Point your DNS A record at the IP above.
       (Cloudflare: keep proxying OFF / grey cloud — TLS must terminate
       at the instance to capture real client fingerprints.)

    2. Wait for DNS to propagate, then bootstrap the instance:

       cd $(git rev-parse --show-toplevel)
       gcloud compute scp infra/scripts/bootstrap.sh \
         infra/scripts/renew-cert.sh \
         infra/systemd/tls-cert-renew.service \
         infra/systemd/tls-cert-renew.timer \
         ${google_compute_instance.this.name}:~/ \
         --zone=${var.zone} --project=${var.project}

       gcloud compute ssh ${google_compute_instance.this.name} \
         --zone=${var.zone} --project=${var.project} \
         --command='sudo bash ~/bootstrap.sh <DOMAIN> <EMAIL> [<DOCKER_IMAGE>]'

       Example:
       sudo bash ~/bootstrap.sh tlsfingerprint.com you@example.com gcr.io/scrolller/tlsfingerprint:latest

    3. Verify:
       echo | openssl s_client -servername <DOMAIN> -connect <DOMAIN>:443 2>/dev/null | openssl x509 -noout -dates
  EOT
}
