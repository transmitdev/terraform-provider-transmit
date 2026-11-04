resource "transmit_heartbeat" "nightly_backup" {
  name                 = "nightly-backup"
  configuration_id     = transmit_oncall_configuration.prod.id
  escalation_policy_id = transmit_oncall_escalation_policy.default.id
  interval_seconds     = 86400
  grace_seconds        = 1800
}

# Ping from the job: curl -X POST -H "Authorization: Bearer $TRANSMIT_API_KEY" <ping_url>
output "backup_ping_url" {
  value = transmit_heartbeat.nightly_backup.ping_url
}
