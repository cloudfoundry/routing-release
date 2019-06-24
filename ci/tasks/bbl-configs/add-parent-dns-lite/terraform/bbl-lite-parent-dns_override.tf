resource "google_dns_record_set" "parent_dns_pointer" {
  name = "*.${var.env_id}.routing.cf-app.com."
  type = "A"
  ttl  = 300

  managed_zone = "routing-team"

  rrdatas = ["${google_compute_address.bosh-director-ip.address}"]
}
